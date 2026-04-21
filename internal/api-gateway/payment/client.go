package payment

import (
	"context"
	"time"

	"github.com/LucasLCabral/payment-service/pkg/payment"
	"github.com/LucasLCabral/payment-service/protog/common"
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
		Currency:       currencyToProto(in.Currency),
		PayerId:        in.PayerID.String(),
		PayeeId:        in.PayeeID.String(),
		Description:    in.Description,
	})
	if err != nil {
		return nil, err
	}

	pid, err := uuid.Parse(resp.GetPaymentId())
	if err != nil {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339, resp.GetCreatedAt())
	if err != nil {
		return nil, err
	}

	return &payment.Payment{
		ID:        pid,
		Status:    statusFromProto(resp.GetStatus()),
		CreatedAt: t,
	}, nil
}

func currencyToProto(c payment.Currency) common.Currency {
	switch c {
	case payment.CurrencyBRL:
		return common.Currency_CURRENCY_BRL
	case payment.CurrencyUSD:
		return common.Currency_CURRENCY_USD
	default:
		return common.Currency_CURRENCY_UNSPECIFIED
	}
}

func statusFromProto(s common.PaymentStatus) payment.PaymentStatus {
	switch s {
	case common.PaymentStatus_PAYMENT_STATUS_PENDING:
		return payment.StatusPending
	case common.PaymentStatus_PAYMENT_STATUS_SETTLED:
		return payment.StatusSettled
	case common.PaymentStatus_PAYMENT_STATUS_DECLINED:
		return payment.StatusDeclined
	case common.PaymentStatus_PAYMENT_STATUS_FAILED:
		return payment.StatusFailed
	default:
		return payment.StatusUnspecified
	}
}
