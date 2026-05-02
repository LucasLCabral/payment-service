package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LucasLCabral/payment-service/internal/ledger"
	ledgerpostgres "github.com/LucasLCabral/payment-service/internal/ledger/repository/postgres"
	"github.com/LucasLCabral/payment-service/pkg/config"
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

	db, err := database.Connect(ctx, database.Config{
		Host:     config.Get("LEDGER_DB_HOST", "localhost"),
		Port:     config.MustInt("LEDGER_DB_PORT", 5433),
		User:     config.Get("LEDGER_DB_USER", "ledger_user"),
		Password: config.Get("LEDGER_DB_PASSWORD", "ledger_pass"),
		Database: config.Get("LEDGER_DB_NAME", "ledger_db"),
		SSLMode:  config.Get("LEDGER_DB_SSLMODE", "disable"),
	})
	if err != nil {
		log.Error(ctx, "database connection failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	cbDB := database.NewCBDatabase(db, "ledger-service")

	rabbitURL := config.Get("RABBITMQ_URL", "")

	if rabbitURL == "" {
		log.Error(ctx, "RABBITMQ_URL is required")
		os.Exit(1)
	}

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

	monitoringHandler := monitoring.NewHandler(log)
	monitoringHandler.RegisterCircuitBreaker(pub)
	monitoringHandler.RegisterCircuitBreaker(cbDB)

	monMux := http.NewServeMux()
	monMux.HandleFunc("GET /health", monitoringHandler.Health)
	monMux.HandleFunc("GET /healthz", monitoringHandler.Health)
	monMux.HandleFunc("GET /circuit-breakers", monitoringHandler.CircuitBreakerStatus)

	monAddr := ":" + config.Get("LEDGER_HTTP_PORT", "8082")
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

	log.Info(ctx, "ledger-service up", "queue", ledger.Queue)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Info(ctx, "shutdown signal received")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := monSrv.Shutdown(shutdownCtx); err != nil {
		log.Error(context.Background(), "monitoring HTTP shutdown", "err", err)
	}
	log.Info(ctx, "ledger-service stopped")
}

