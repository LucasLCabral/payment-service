package payment

import (
	"context"
	"time"

	"github.com/LucasLCabral/payment-service/pkg/payment"
	"github.com/LucasLCabral/payment-service/pkg/protoconv"
	pb "github.com/LucasLCabral/payment-service/protog/payment"
	"github.com/google/uuid"
	"google.golang.org/grpc"
)

type Client struct {
	api pb.PaymentServiceClient
}

func New(conn grpc.ClientConnInterface) *Client {
	return &Client{api: pb.NewPaymentServiceClient(conn)}
}

func (c *Client) CreatePayment(ctx context.Context, in *payment.CreatePaymentRequest) (*payment.Payment, error) {
	resp, err := c.api.CreatePayment(ctx, &pb.CreatePaymentRequest{
		IdempotencyKey: in.IdempotencyKey.String(),
		AmountCents:    in.AmountCents,
		Currency:       protoconv.CurrencyToProto(in.Currency),
		PayerId:        in.PayerID.String(),
		PayeeId:        in.PayeeID.String(),
		Description:    in.Description,
	})
	if err != nil {
		return nil, err
	}

	paymentID, err := uuid.Parse(resp.GetPaymentId())
	if err != nil {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339, resp.GetCreatedAt())
	if err != nil {
		return nil, err
	}

	return &payment.Payment{
		ID:        paymentID,
		Status:    protoconv.StatusFromProto(resp.GetStatus()),
		CreatedAt: t,
	}, nil
}

func (c *Client) GetPayment(ctx context.Context, req *payment.GetPaymentRequest) (*payment.Payment, error) {
	resp, err := c.api.GetPayment(ctx, &pb.GetPaymentRequest{
		PaymentId: req.PaymentID.String(),
	})
	if err != nil {
		return nil, err
	}

	paymentID, err := uuid.Parse(resp.GetPaymentId())
	if err != nil {
		return nil, err
	}
	created, _ := time.Parse(time.RFC3339, resp.GetCreatedAt())
	updated, _ := time.Parse(time.RFC3339, resp.GetUpdatedAt())

	return &payment.Payment{
		ID:            paymentID,
		AmountCents:   resp.GetAmountCents(),
		Currency:      protoconv.CurrencyFromProtoUnsafe(resp.GetCurrency()),
		Status:        protoconv.StatusFromProto(resp.GetStatus()),
		DeclineReason: resp.GetDeclineReason(),
		CreatedAt:     created,
		UpdatedAt:     updated,
	}, nil
}
