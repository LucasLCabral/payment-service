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

	log := logger.New("payment-service")

	ctx = trace.EnsureTraceID(ctx)

	log.Info(ctx, "Starting Payment Service",
		"grpc_port", getEnv("PAYMENT_GRPC_PORT", "9090"),
		"environment", getEnv("ENVIRONMENT", "development"),
	)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// TODO: Implementar servidor gRPC (Fase 5)
	// TODO: Implementar repository com PostgreSQL (Fase 3)
	// TODO: Implementar outbox publisher (Fase 6)
	// TODO: Implementar consumer de ledger.settled.* (Fase 5)

	log.Info(ctx, "Payment Service ready")

	// graceful shutdown
	sig := <-sigChan
	log.Info(ctx, "Received shutdown signal", "signal", sig)

	log.Info(ctx, "Payment Service stopped gracefully")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
