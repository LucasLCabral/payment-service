package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/LucasLCabral/payment-service/internal/payment/grpcsvc"
	"github.com/LucasLCabral/payment-service/internal/payment/repository/postgres"
	"github.com/LucasLCabral/payment-service/internal/payment/service"
	"github.com/LucasLCabral/payment-service/pkg/database"
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

	port, err := strconv.Atoi(getEnv("PAYMENT_DB_PORT", "5432"))
	if err != nil {
		log.Error(ctx, "invalid PAYMENT_DB_PORT", "err", err)
		os.Exit(1)
	}

	dbCfg := database.Config{
		Host:     getEnv("PAYMENT_DB_HOST", "localhost"),
		Port:     port,
		User:     getEnv("PAYMENT_DB_USER", "payment_user"),
		Password: getEnv("PAYMENT_DB_PASSWORD", "payment_pass"),
		Database: getEnv("PAYMENT_DB_NAME", "payment_db"),
		SSLMode:  getEnv("PAYMENT_DB_SSLMODE", "disable"),
	}

	db, err := database.Connect(ctx, dbCfg)
	if err != nil {
		log.Error(ctx, "database connection failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	repo := &postgres.PaymentRepository{DB: db}
	txRunner := database.NewTransactor(db)
	svc := service.NewPayment(txRunner, repo)

	addr := ":" + getEnv("PAYMENT_GRPC_PORT", "9090")
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Error(ctx, "failed to listen", "addr", addr, "err", err)
		os.Exit(1)
	}

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(grpctrace.UnaryServerInterceptor()),
	)
	payment.RegisterPaymentServiceServer(srv, &grpcsvc.Server{Svc: svc})

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
