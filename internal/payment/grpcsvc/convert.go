package grpcsvc

import (
	"time"

	"github.com/LucasLCabral/payment-service/pkg/payment"
	"github.com/LucasLCabral/payment-service/pkg/protoconv"
	pb "github.com/LucasLCabral/payment-service/protog/payment"
	"github.com/google/uuid"
)

func createRequestToModel(req *pb.CreatePaymentRequest) (*payment.CreatePaymentRequest, error) {
	cur, ok := protoconv.CurrencyFromProto(req.GetCurrency())
	if !ok {
		return nil, payment.ErrCurrencyRequired
	}
	return payment.ParseCreatePaymentRequest(
		req.GetIdempotencyKey(),
		req.GetAmountCents(),
		cur,
		req.GetPayerId(),
		req.GetPayeeId(),
		req.GetDescription(),
	)
}

func paymentToCreateResponse(p *payment.Payment) *pb.CreatePaymentResponse {
	return &pb.CreatePaymentResponse{
		PaymentId: p.ID.String(),
		Status:    protoconv.StatusToProto(p.Status),
		CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func getRequestToModel(req *pb.GetPaymentRequest) (*payment.GetPaymentRequest, error) {
	pid, err := uuid.Parse(req.GetPaymentId())
	if err != nil {
		return nil, err
	}
	return &payment.GetPaymentRequest{PaymentID: pid}, nil
}

func paymentToGetResponse(p *payment.Payment) *pb.GetPaymentResponse {
	return &pb.GetPaymentResponse{
		PaymentId:     p.ID.String(),
		Status:        protoconv.StatusToProto(p.Status),
		AmountCents:   p.AmountCents,
		Currency:      protoconv.CurrencyToProto(p.Currency),
		CreatedAt:     p.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     p.UpdatedAt.UTC().Format(time.RFC3339),
		DeclineReason: p.DeclineReason,
	}
}
