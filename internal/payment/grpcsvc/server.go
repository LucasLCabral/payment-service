package grpcsvc

import (
	"context"
	"database/sql"
	"errors"
	"time"

	model "github.com/LucasLCabral/payment-service/internal/payment/models"
	"github.com/LucasLCabral/payment-service/protog/payment"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PaymentService interface {
	CreatePayment(ctx context.Context, in *model.CreatePaymentRequest) (*model.Payment, error)
	GetPayment(ctx context.Context, req *model.GetPaymentRequest) (*model.Payment, error)
}

type Server struct {
	payment.UnimplementedPaymentServiceServer
	Svc PaymentService
}

func (s *Server) CreatePayment(ctx context.Context, req *payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	if s.Svc == nil {
		return nil, status.Error(codes.Unavailable, "service not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	in, err := CreatePaymentRequestToModel(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	res, err := s.Svc.CreatePayment(ctx, in)
	if err != nil {
		if model.IsValidationError(err) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return PaymentToCreateResponse(res), nil
}

func (s *Server) GetPayment(ctx context.Context, req *payment.GetPaymentRequest) (*payment.GetPaymentResponse, error) {
	if s.Svc == nil {
		return nil, status.Error(codes.Unavailable, "service not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	in, err := GetPaymentRequestToModel(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	res, err := s.Svc.GetPayment(ctx, in)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, status.Error(codes.NotFound, "payment not found")
		}
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return PaymentToGetResponse(res), nil
}
