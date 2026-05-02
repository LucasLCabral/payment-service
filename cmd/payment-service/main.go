package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LucasLCabral/payment-service/internal/payment/grpcsvc"
	"github.com/LucasLCabral/payment-service/internal/payment/outbox"
	"github.com/LucasLCabral/payment-service/internal/payment/repository/postgres"
	"github.com/LucasLCabral/payment-service/internal/payment/service"
	"github.com/LucasLCabral/payment-service/internal/payment/settlement"
	"github.com/LucasLCabral/payment-service/pkg/config"
	"github.com/LucasLCabral/payment-service/pkg/database"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/messaging"
	"github.com/LucasLCabral/payment-service/pkg/monitoring"
	"github.com/LucasLCabral/payment-service/pkg/notify"
	"github.com/LucasLCabral/payment-service/pkg/telemetry"
	"github.com/LucasLCabral/payment-service/pkg/trace"
	pb "github.com/LucasLCabral/payment-service/protog/payment"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
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

	db, err := database.Connect(ctx, database.Config{
		Host:     config.Get("PAYMENT_DB_HOST", "localhost"),
		Port:     config.MustInt("PAYMENT_DB_PORT", 5432),
		User:     config.Get("PAYMENT_DB_USER", "payment_user"),
		Password: config.Get("PAYMENT_DB_PASSWORD", "payment_pass"),
		Database: config.Get("PAYMENT_DB_NAME", "payment_db"),
		SSLMode:  config.Get("PAYMENT_DB_SSLMODE", "disable"),
	})
	if err != nil {
		log.Error(ctx, "database connection failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	cbDB := database.NewCBDatabase(db, "payment-service")
	repo := &postgres.PaymentRepository{DB: db}
	audit := &postgres.AuditRepository{DB: db}
	txRunner := database.NewTransactor(db)
	svc := service.NewPayment(txRunner, repo)

	var rabbit *messaging.Publisher
	var notifier settlement.PaymentStatusNotifier

	if rabbitURL := config.Get("RABBITMQ_URL", ""); rabbitURL != "" {
		var err error
		rabbit, err = messaging.NewPublisher(ctx, messaging.Config{URL: rabbitURL})
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

		if redisURL := config.Get("REDIS_URL", ""); redisURL != "" {
			rdb, err := notify.ConnectRedis(ctx, redisURL)
			if err != nil {
				log.Warn(ctx, "redis connection failed, settlement will not push WS updates", "err", err)
			} else {
				defer rdb.Close()
				notifier = notify.NewRedisPublisher(rdb)
				log.Info(ctx, "redis publisher enabled for payment status", "addr", redisURL)
			}
		}

		settlementHandler := settlement.NewHandler(txRunner, repo, audit, log, notifier)
		go messaging.RunConsumer(ctx, messaging.Config{URL: rabbitURL}, settlement.Queue, log, settlementHandler.HandleMessage)

		log.Info(ctx, "outbox publisher + settlement consumer enabled", "rabbitmq", rabbitURL)
	} else {
		log.Warn(ctx, "RABBITMQ_URL not set, outbox publisher and settlement consumer disabled")
	}

	addr := ":" + config.Get("PAYMENT_GRPC_PORT", "9090")
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Error(ctx, "failed to listen", "addr", addr, "err", err)
		os.Exit(1)
	}

	srv := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 30 * time.Second,
			Time:              20 * time.Second,
			Timeout:           5 * time.Second,
		}),
	)
	pb.RegisterPaymentServiceServer(srv, &grpcsvc.Server{Svc: svc})

	go func() {
		log.Info(ctx, "gRPC listening", "addr", addr)
		if err := srv.Serve(lis); err != nil {
			log.Error(context.Background(), "gRPC server stopped", "err", err)
		}
	}()

	monitoringHandler := monitoring.NewHandler(log)
	metricsCollector := monitoring.NewMetricsCollector(db)

	if rabbit != nil {
		monitoringHandler.RegisterCircuitBreaker(rabbit)
	}
	if redisNotifier, ok := notifier.(*notify.RedisPublisher); ok && redisNotifier != nil {
		monitoringHandler.RegisterCircuitBreaker(redisNotifier)
	}
	monitoringHandler.RegisterCircuitBreaker(cbDB)

	monMux := http.NewServeMux()
	monMux.HandleFunc("GET /health", monitoringHandler.Health)
	monMux.HandleFunc("GET /healthz", monitoringHandler.Health)
	monMux.HandleFunc("GET /circuit-breakers", monitoringHandler.CircuitBreakerStatus)
	monMux.HandleFunc("GET /metrics", metricsCollector.MetricsHandler())
	monMux.HandleFunc("GET /metrics/prometheus", metricsCollector.PrometheusMetricsHandler())
	monMux.HandleFunc("GET /performance", func(w http.ResponseWriter, r *http.Request) {
		status := metricsCollector.PerformanceAlert()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	monAddr := ":" + config.Get("PAYMENT_HTTP_PORT", "8081")
	monSrv := &http.Server{
		Addr:         monAddr,
		Handler:      monMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	go func() {
		log.Info(ctx, "monitoring HTTP listening", "addr", monAddr)
		if err := monSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(context.Background(), "monitoring HTTP stopped", "err", err)
		}
	}()

	log.Info(ctx, "payment-service up", "grpc", addr)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Info(ctx, "shutdown signal received")
	cancel()
	srv.GracefulStop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := monSrv.Shutdown(shutdownCtx); err != nil {
		log.Error(context.Background(), "monitoring HTTP shutdown", "err", err)
	}
	log.Info(ctx, "payment-service stopped")
}
