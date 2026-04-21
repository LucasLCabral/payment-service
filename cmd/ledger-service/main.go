package main

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/LucasLCabral/payment-service/internal/ledger"
	ledgerpostgres "github.com/LucasLCabral/payment-service/internal/ledger/repository/postgres"
	"github.com/LucasLCabral/payment-service/pkg/database"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/messaging"
	"github.com/LucasLCabral/payment-service/pkg/trace"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log := logger.New("ledger-service")
	ctx = trace.EnsureTraceID(ctx)

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

	cons, err := messaging.NewConsumer(ctx, messaging.Config{URL: rabbitURL})
	if err != nil {
		log.Error(ctx, "rabbitmq consumer connection failed", "err", err)
		os.Exit(1)
	}
	defer cons.Close()

	repo := &ledgerpostgres.Repository{DB: db}
	tx := database.NewTransactor(db)
	svc := ledger.NewService(tx, repo, pub, log)
	handler := ledger.NewHandler(svc, log)

	go func() {
		log.Info(ctx, "consuming queue", "queue", ledger.Queue)
		if err := cons.Consume(ctx, ledger.Queue, handler.HandleMessage); err != nil {
			log.Error(ctx, "consumer stopped", "err", err)
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
