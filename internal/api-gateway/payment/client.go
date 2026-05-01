package payment

import (
	"context"
	"time"

	"github.com/LucasLCabral/payment-service/pkg/payment"
	"github.com/LucasLCabral/payment-service/pkg/protoconv"
	pb "github.com/LucasLCabral/payment-service/protog/payment"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const grpcMaxAttempts = 3

type Client struct {
	api pb.PaymentServiceClient
}

func New(conn grpc.ClientConnInterface) *Client {
	return &Client{api: pb.NewPaymentServiceClient(conn)}
}

func (c *Client) CreatePayment(ctx context.Context, in *payment.CreatePaymentRequest) (*payment.Payment, error) {
	var lastErr error
	for attempt := 1; attempt <= grpcMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resp, err := c.api.CreatePayment(ctx, &pb.CreatePaymentRequest{
			IdempotencyKey: in.IdempotencyKey.String(),
			AmountCents:    in.AmountCents,
			Currency:       protoconv.CurrencyToProto(in.Currency),
			PayerId:        in.PayerID.String(),
			PayeeId:        in.PayeeID.String(),
			Description:    in.Description,
		})
		if err != nil {
			lastErr = err
			if !grpcRetryable(err) || attempt == grpcMaxAttempts {
				return nil, err
			}
			if err := sleepGRPCBackoff(ctx, attempt); err != nil {
				return nil, err
			}
			continue
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
	return nil, lastErr
}

func (c *Client) GetPayment(ctx context.Context, req *payment.GetPaymentRequest) (*payment.Payment, error) {
	var lastErr error
	for attempt := 1; attempt <= grpcMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resp, err := c.api.GetPayment(ctx, &pb.GetPaymentRequest{
			PaymentId: req.PaymentID.String(),
		})
		if err != nil {
			lastErr = err
			if !grpcRetryable(err) || attempt == grpcMaxAttempts {
				return nil, err
			}
			if err := sleepGRPCBackoff(ctx, attempt); err != nil {
				return nil, err
			}
			continue
		}

		paymentID, err := uuid.Parse(resp.GetPaymentId())
		if err != nil {
			return nil, err
		}
		payerID, err := uuid.Parse(resp.GetPayerId())
		if err != nil {
			return nil, err
		}
		payeeID, err := uuid.Parse(resp.GetPayeeId())
		if err != nil {
			return nil, err
		}
		created, _ := time.Parse(time.RFC3339, resp.GetCreatedAt())
		updated, _ := time.Parse(time.RFC3339, resp.GetUpdatedAt())

		return &payment.Payment{
			ID:            paymentID,
			PayerID:       payerID,
			PayeeID:       payeeID,
			AmountCents:   resp.GetAmountCents(),
			Currency:      protoconv.CurrencyFromProtoUnsafe(resp.GetCurrency()),
			Status:        protoconv.StatusFromProto(resp.GetStatus()),
			DeclineReason: resp.GetDeclineReason(),
			CreatedAt:     created,
			UpdatedAt:     updated,
		}, nil
	}
	return nil, lastErr
}

func grpcRetryable(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return true
	}
	switch st.Code() {
	case codes.Unavailable, codes.ResourceExhausted, codes.Aborted, codes.DeadlineExceeded:
		return true
	default:
		return false
	}
}

func sleepGRPCBackoff(ctx context.Context, attempt int) error {
	d := time.Duration(attempt*attempt) * 50 * time.Millisecond
	if d > 600*time.Millisecond {
		d = 600 * time.Millisecond
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
