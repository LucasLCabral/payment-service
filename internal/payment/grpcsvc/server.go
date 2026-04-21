package grpcsvc

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/LucasLCabral/payment-service/pkg/telemetry"
	"github.com/LucasLCabral/payment-service/pkg/payment"
	pb "github.com/LucasLCabral/payment-service/protog/payment"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PaymentService interface {
	CreatePayment(ctx context.Context, in *payment.CreatePaymentRequest) (*payment.Payment, error)
	GetPayment(ctx context.Context, req *payment.GetPaymentRequest) (*payment.Payment, error)
}

type Server struct {
	pb.UnimplementedPaymentServiceServer
	Svc PaymentService
}

func (s *Server) CreatePayment(ctx context.Context, req *pb.CreatePaymentRequest) (*pb.CreatePaymentResponse, error) {
	if s.Svc == nil {
		return nil, status.Error(codes.Unavailable, "service not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	in, err := createRequestToModel(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	res, err := s.Svc.CreatePayment(ctx, in)
	if err != nil {
		if payment.IsValidationError(err) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	telemetry.AnnotatePaymentID(ctx, res.ID.String())
	return paymentToCreateResponse(res), nil
}

func (s *Server) GetPayment(ctx context.Context, req *pb.GetPaymentRequest) (*pb.GetPaymentResponse, error) {
	if s.Svc == nil {
		return nil, status.Error(codes.Unavailable, "service not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	in, err := getRequestToModel(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	telemetry.AnnotatePaymentID(ctx, in.PaymentID.String())

	res, err := s.Svc.GetPayment(ctx, in)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, status.Error(codes.NotFound, "payment not found")
		}
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return paymentToGetResponse(res), nil
}
