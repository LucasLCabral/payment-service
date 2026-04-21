package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpapi "github.com/LucasLCabral/payment-service/internal/api-gateway/http"
	"github.com/LucasLCabral/payment-service/internal/api-gateway/payment"
	"github.com/LucasLCabral/payment-service/internal/api-gateway/ws"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/telemetry"
	"github.com/LucasLCabral/payment-service/pkg/trace"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log := logger.New("api-gateway")
	ctx = trace.EnsureTraceID(ctx)

	otelShutdown, err := telemetry.Init(ctx, "api-gateway")
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

	paymentAddr := getEnv("PAYMENT_GRPC_ADDR", "localhost:9090")
	var pay httpapi.PaymentService
	grpcConn, err := grpc.NewClient(
		paymentAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		log.Warn(ctx, "gRPC client failed", "addr", paymentAddr, "err", err)
	} else {
		defer grpcConn.Close()
		pay = payment.New(grpcConn)
	}

	reg := ws.NewRegistry()

	paymentsHandler := httpapi.NewPaymentsHandler(log, pay)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /payments", paymentsHandler.Create)
	mux.HandleFunc("GET /payments/{id}", paymentsHandler.Get)
	mux.Handle("GET /ws", &ws.Handler{Reg: reg, Log: log})

	logged := httpapi.LoggingMiddleware(log)(mux)
	handler := otelhttp.NewHandler(logged, "api-gateway")

	httpAddr := ":" + getEnv("API_GATEWAY_PORT", "8080")
	srv := &http.Server{
		Addr:    httpAddr,
		Handler: handler,
	}

	go func() {
		log.Info(ctx, "http listening", "addr", httpAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(ctx, "http server error", "err", err)
		}
	}()

	log.Info(ctx, "API Gateway up",
		"environment", getEnv("ENVIRONMENT", "development"),
		"payment_grpc", paymentAddr,
	)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Info(ctx, "shutdown signal received")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error(ctx, "http shutdown", "err", err)
	}
	log.Info(ctx, "api-gateway stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
