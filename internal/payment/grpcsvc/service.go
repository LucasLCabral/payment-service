package grpcsvc

import (
	"context"

	"github.com/LucasLCabral/payment-service/protog/payment"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	payment.UnimplementedPaymentServiceServer
}

func (s *Server) CreatePayment(ctx context.Context, req *payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	return nil, status.Error(codes.Unimplemented, "CreatePayment not implemented")
}

func (s *Server) GetPayment(ctx context.Context, req *payment.GetPaymentRequest) (*payment.GetPaymentResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetPayment not implemented")
}
