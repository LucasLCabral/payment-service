package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/LucasLCabral/payment-service/internal/ledger"
	ledgerpostgres "github.com/LucasLCabral/payment-service/internal/ledger/repository/postgres"
	"github.com/LucasLCabral/payment-service/pkg/database"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/messaging"
	"github.com/LucasLCabral/payment-service/pkg/monitoring"
	"github.com/LucasLCabral/payment-service/pkg/telemetry"
	"github.com/LucasLCabral/payment-service/pkg/trace"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log := logger.New("ledger-service")
	ctx = trace.EnsureTraceID(ctx)

	otelShutdown, err := telemetry.Init(ctx, "ledger-service")
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

	port, err := strconv.Atoi(getEnv("LEDGER_DB_PORT", "5433"))
	if err != nil {
		log.Error(ctx, "invalid LEDGER_DB_PORT", "err", err)
		os.Exit(1)
	}

	db, err := database.Connect(ctx, database.Config{
		Host:     getEnv("LEDGER_DB_HOST", "localhost"),
		Port:     port,
		User:     getEnv("LEDGER_DB_USER", "ledger_user"),
		Password: getEnv("LEDGER_DB_PASSWORD", "ledger_pass"),
		Database: getEnv("LEDGER_DB_NAME", "ledger_db"),
		SSLMode:  getEnv("LEDGER_DB_SSLMODE", "disable"),
	})
	if err != nil {
		log.Error(ctx, "database connection failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	cbDB := database.NewCBDatabase(db, "ledger-service")

	rabbitURL := getEnv("RABBITMQ_URL", "amqp://rabbit_user:rabbit_pass@localhost:5672/")

	pub, err := messaging.NewPublisher(ctx, messaging.Config{URL: rabbitURL})
	if err != nil {
		log.Error(ctx, "rabbitmq publisher connection failed", "err", err)
		os.Exit(1)
	}
	defer pub.Close()

	if err := ledger.DeclareTopology(pub.Channel()); err != nil {
		log.Error(ctx, "rabbitmq topology setup failed", "err", err)
		os.Exit(1)
	}

	repo := &ledgerpostgres.Repository{DB: db}
	tx := database.NewTransactor(db)
	svc := ledger.NewService(tx, repo, pub, log)
	handler := ledger.NewHandler(svc, log)

	go messaging.RunConsumer(ctx, messaging.Config{URL: rabbitURL}, ledger.Queue, log, handler.HandleMessage)

	// Monitoring endpoints
	monitoringHandler := monitoring.NewHandler(log)
	monitoringHandler.RegisterCircuitBreaker(pub)
	monitoringHandler.RegisterCircuitBreaker(cbDB)

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /health", monitoringHandler.Health)
		mux.HandleFunc("GET /healthz", monitoringHandler.Health) // K8s health checks (no logging)
		mux.HandleFunc("GET /circuit-breakers", monitoringHandler.CircuitBreakerStatus)
		
		httpAddr := ":" + getEnv("LEDGER_HTTP_PORT", "8082")
		log.Info(ctx, "monitoring HTTP listening", "addr", httpAddr)
		if err := http.ListenAndServe(httpAddr, mux); err != nil {
			log.Error(context.Background(), "monitoring HTTP server stopped", "err", err)
		}
	}()

	log.Info(ctx, "ledger-service up",
		"rabbitmq", rabbitURL,
		"queue", ledger.Queue,
	)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Info(ctx, "shutdown signal received")
	cancel()
	log.Info(ctx, "ledger-service stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
