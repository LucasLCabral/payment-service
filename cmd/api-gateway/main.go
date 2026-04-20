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

	log := logger.New("api-gateway")

	ctx = trace.EnsureTraceID(ctx)

	log.Info(ctx, "Starting API Gateway",
		"port", getEnv("API_GATEWAY_PORT", "8080"),
		"environment", getEnv("ENVIRONMENT", "development"),
	)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// TODO: Implementar servidor HTTP/WebSocket (Fase 4)
	// TODO: Implementar cliente gRPC para Payment Service (Fase 4)
	// TODO: Implementar Redis Pub/Sub (Fase 4)

	log.Info(ctx, "API Gateway service ready")

	// graceful shutdown
	sig := <-sigChan
	log.Info(ctx, "Received shutdown signal", "signal", sig)

	log.Info(ctx, "API Gateway service stopped gracefully")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
