package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/LucasLCabral/payment-service/pkg/grpctrace"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/trace"
	"github.com/LucasLCabral/payment-service/protog/payment"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log := logger.New("api-gateway")
	ctx = trace.EnsureTraceID(ctx)

	paymentAddr := getEnv("PAYMENT_GRPC_ADDR", "localhost:9090")
	conn, err := grpc.NewClient(
		paymentAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(grpctrace.UnaryClientInterceptor()),
	)
	if err != nil {
		log.Warn(ctx, "gRPC client não conectou ao payment-service (subir com payment-service)", "addr", paymentAddr, "err", err)
	} else {
		defer conn.Close()
		client := payment.NewPaymentServiceClient(conn)
		callCtx := trace.WithTraceID(ctx, "smoke-"+trace.NewTraceID())
		_, err := client.GetPayment(callCtx, &payment.GetPaymentRequest{PaymentId: "00000000-0000-0000-0000-000000000000"})
		if err != nil {
			log.Info(ctx, "chamada gRPC de smoke (esperado Unimplemented até Fase 5)", "err", err.Error())
		}
	}

	log.Info(ctx, "API Gateway up",
		"port", getEnv("API_GATEWAY_PORT", "8080"),
		"environment", getEnv("ENVIRONMENT", "development"),
		"payment_grpc", paymentAddr,
	)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	log.Info(ctx, "shutdown signal received")
	log.Info(ctx, "api-gateway stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
