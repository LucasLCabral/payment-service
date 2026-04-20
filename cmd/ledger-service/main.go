package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/trace"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log := logger.New("ledger-service")

	ctx = trace.EnsureTraceID(ctx)

	log.Info(ctx, "Starting Ledger Service",
		"environment", getEnv("ENVIRONMENT", "development"),
	)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// TODO: Implementar consumer de payment.created (Fase 7)
	// TODO: Implementar repository com PostgreSQL (Fase 3)
	// TODO: Implementar lógica de aceitar/recusar (Fase 7)
	// TODO: Implementar publisher de ledger.settled.* (Fase 7)

	log.Info(ctx, "Ledger Service ready")

	// graceful shutdown
	sig := <-sigChan
	log.Info(ctx, "Received shutdown signal", "signal", sig)

	log.Info(ctx, "Ledger Service stopped gracefully")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
