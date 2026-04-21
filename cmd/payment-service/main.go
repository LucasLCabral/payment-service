package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/LucasLCabral/payment-service/internal/payment/grpcsvc"
	"github.com/LucasLCabral/payment-service/internal/payment/outbox"
	"github.com/LucasLCabral/payment-service/internal/payment/repository/postgres"
	"github.com/LucasLCabral/payment-service/internal/payment/service"
	"github.com/LucasLCabral/payment-service/pkg/database"
	"github.com/LucasLCabral/payment-service/pkg/grpctrace"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/messaging"
	"github.com/LucasLCabral/payment-service/pkg/trace"
	pb "github.com/LucasLCabral/payment-service/protog/payment"
	"google.golang.org/grpc"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log := logger.New("payment-service")
	ctx = trace.EnsureTraceID(ctx)

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

		pub := outbox.NewPublisher(db, rabbit, log)
		go pub.Run(ctx)
		log.Info(ctx, "outbox publisher enabled", "rabbitmq", rabbitURL)
	} else {
		log.Warn(ctx, "RABBITMQ_URL not set, outbox publisher disabled")
	}

	addr := ":" + getEnv("PAYMENT_GRPC_PORT", "9090")
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Error(ctx, "failed to listen", "addr", addr, "err", err)
		os.Exit(1)
	}

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(grpctrace.UnaryServerInterceptor()),
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
