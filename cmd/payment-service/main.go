package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/LucasLCabral/payment-service/internal/payment/grpcsvc"
	"github.com/LucasLCabral/payment-service/pkg/grpctrace"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/trace"
	"github.com/LucasLCabral/payment-service/protog/payment"
	"google.golang.org/grpc"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log := logger.New("payment-service")
	ctx = trace.EnsureTraceID(ctx)

	addr := ":" + getEnv("PAYMENT_GRPC_PORT", "9090")
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Error(ctx, "failed to listen", "addr", addr, "err", err)
		os.Exit(1)
	}

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(grpctrace.UnaryServerInterceptor()),
	)
	payment.RegisterPaymentServiceServer(srv, &grpcsvc.Server{})

	go func() {
		log.Info(ctx, "gRPC listening", "addr", addr)
		if err := srv.Serve(lis); err != nil {
			log.Error(context.Background(), "gRPC server stopped", "err", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	log.Info(ctx, "shutdown signal received")
	srv.GracefulStop()
	log.Info(ctx, "payment-service stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
