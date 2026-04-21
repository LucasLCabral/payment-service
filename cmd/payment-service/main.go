package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/LucasLCabral/payment-service/internal/payment/grpcsvc"
	"github.com/LucasLCabral/payment-service/internal/payment/outbox"
	"github.com/LucasLCabral/payment-service/internal/payment/repository/postgres"
	"github.com/LucasLCabral/payment-service/internal/payment/service"
	"github.com/LucasLCabral/payment-service/internal/payment/settlement"
	"github.com/LucasLCabral/payment-service/pkg/database"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/notify"
	"github.com/LucasLCabral/payment-service/pkg/telemetry"
	"github.com/LucasLCabral/payment-service/pkg/messaging"
	"github.com/LucasLCabral/payment-service/pkg/trace"
	pb "github.com/LucasLCabral/payment-service/protog/payment"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log := logger.New("payment-service")
	ctx = trace.EnsureTraceID(ctx)

	otelShutdown, err := telemetry.Init(ctx, "payment-service")
	if err != nil {
		log.Error(ctx, "otel setup failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		shutdownOtelCtx, shutdownOtelCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownOtelCancel()
		if err := otelShutdown(shutdownOtelCtx); err != nil {
			log.Error(context.Background(), "otel shutdown", "err", err)
		}
	}()

	port, err := strconv.Atoi(getEnv("PAYMENT_DB_PORT", "5432"))
	if err != nil {
		log.Error(ctx, "invalid PAYMENT_DB_PORT", "err", err)
		os.Exit(1)
	}

	db, err := database.Connect(ctx, database.Config{
		Host:     getEnv("PAYMENT_DB_HOST", "localhost"),
		Port:     port,
		User:     getEnv("PAYMENT_DB_USER", "payment_user"),
		Password: getEnv("PAYMENT_DB_PASSWORD", "payment_pass"),
		Database: getEnv("PAYMENT_DB_NAME", "payment_db"),
		SSLMode:  getEnv("PAYMENT_DB_SSLMODE", "disable"),
	})
	if err != nil {
		log.Error(ctx, "database connection failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	repo := &postgres.PaymentRepository{DB: db}
	txRunner := database.NewTransactor(db)
	svc := service.NewPayment(txRunner, repo)

	if rabbitURL := os.Getenv("RABBITMQ_URL"); rabbitURL != "" {
		rabbit, err := messaging.NewPublisher(ctx, messaging.Config{URL: rabbitURL})
		if err != nil {
			log.Error(ctx, "rabbitmq connection failed", "err", err)
			os.Exit(1)
		}
		defer rabbit.Close()

		if err := outbox.DeclareExchange(rabbit); err != nil {
			log.Error(ctx, "rabbitmq exchange declare failed", "err", err)
			os.Exit(1)
		}

		if err := settlement.DeclareTopology(rabbit.Channel()); err != nil {
			log.Error(ctx, "settlement topology setup failed", "err", err)
			os.Exit(1)
		}

		pub := outbox.NewPublisher(db, rabbit, log)
		go pub.Run(ctx)

		cons, err := messaging.NewConsumer(ctx, messaging.Config{URL: rabbitURL})
		if err != nil {
			log.Error(ctx, "rabbitmq consumer connection failed", "err", err)
			os.Exit(1)
		}
		defer cons.Close()

		var notifier settlement.PaymentStatusNotifier
		if redisURL := getEnv("REDIS_URL", ""); redisURL != "" {
			rdb, err := notify.ConnectRedis(ctx, redisURL)
			if err != nil {
				log.Warn(ctx, "redis connection failed, settlement will not push WS updates", "err", err)
			} else {
				defer rdb.Close()
				notifier = notify.NewRedisPublisher(rdb)
				log.Info(ctx, "redis publisher enabled for payment status", "addr", redisURL)
			}
		}

		settlementHandler := settlement.NewHandler(txRunner, repo, log, notifier)
		go func() {
			log.Info(ctx, "consuming settlement queue", "queue", settlement.Queue)
			if err := cons.Consume(ctx, settlement.Queue, settlementHandler.HandleMessage); err != nil {
				log.Error(ctx, "settlement consumer stopped", "err", err)
			}
		}()

		log.Info(ctx, "outbox publisher + settlement consumer enabled", "rabbitmq", rabbitURL)
	} else {
		log.Warn(ctx, "RABBITMQ_URL not set, outbox publisher and settlement consumer disabled")
	}

	addr := ":" + getEnv("PAYMENT_GRPC_PORT", "9090")
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Error(ctx, "failed to listen", "addr", addr, "err", err)
		os.Exit(1)
	}

	srv := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	)
	pb.RegisterPaymentServiceServer(srv, &grpcsvc.Server{Svc: svc})

	go func() {
		log.Info(ctx, "gRPC listening", "addr", addr)
		if err := srv.Serve(lis); err != nil {
			log.Error(context.Background(), "gRPC server stopped", "err", err)
		}
	}()

	log.Info(ctx, "payment-service up", "grpc", addr)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Info(ctx, "shutdown signal received")
	cancel()
	srv.GracefulStop()
	log.Info(ctx, "payment-service stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
