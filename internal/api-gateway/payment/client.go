package payment

import (
	"context"
	"fmt"
	"time"

	"github.com/LucasLCabral/payment-service/pkg/circuitbreaker"
	"github.com/LucasLCabral/payment-service/pkg/payment"
	"github.com/LucasLCabral/payment-service/pkg/protoconv"
	pb "github.com/LucasLCabral/payment-service/protog/payment"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Client struct {
	api pb.PaymentServiceClient
	cb  circuitbreaker.CircuitBreaker
}

func NewClient(conn grpc.ClientConnInterface, serviceName string) *Client {
	config := circuitbreaker.GRPCConfig()

	config.MaxRequests = 10
	config.Timeout = 10 * time.Second

	config.ReadyToTrip = func(counts circuitbreaker.Counts) bool {
		return counts.ConsecutiveFailures >= 10 ||
			(counts.Requests >= 20 && float64(counts.TotalFailures)/float64(counts.Requests) >= 0.70)
	}
	config.IsSuccessful = isGRPCCallSuccessful

	cb := circuitbreaker.NewCircuitBreaker(
		fmt.Sprintf("payment-service-%s", serviceName),
		config,
	)

	return &Client{
		api: pb.NewPaymentServiceClient(conn),
		cb:  cb,
	}
}

func (c *Client) CreatePayment(ctx context.Context, in *payment.CreatePaymentRequest) (*payment.Payment, error) {
	var result *payment.Payment

	err := c.cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		resp, err := c.api.CreatePayment(ctx, &pb.CreatePaymentRequest{
			IdempotencyKey: in.IdempotencyKey.String(),
			AmountCents:    in.AmountCents,
			Currency:       protoconv.CurrencyToProto(in.Currency),
			PayerId:        in.PayerID.String(),
			PayeeId:        in.PayeeID.String(),
			Description:    in.Description,
		})

		if err != nil {
			return err
		}

		paymentID, err := uuid.Parse(resp.GetPaymentId())
		if err != nil {
			return err
		}

		t, err := time.Parse(time.RFC3339, resp.GetCreatedAt())
		if err != nil {
			return err
		}

		result = &payment.Payment{
			ID:        paymentID,
			Status:    protoconv.StatusFromProto(resp.GetStatus()),
			CreatedAt: t,
		}

		return nil
	})

	if err != nil {
		if err == circuitbreaker.ErrOpenState {
			return nil, fmt.Errorf("payment service temporarily unavailable (circuit breaker open): %w", err)
		}
		if err == circuitbreaker.ErrTooManyRequests {
			return nil, fmt.Errorf("payment service overloaded (too many requests): %w", err)
		}
		return nil, err
	}

	return result, nil
}

func (c *Client) GetPayment(ctx context.Context, req *payment.GetPaymentRequest) (*payment.Payment, error) {
	var result *payment.Payment

	err := c.cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		resp, err := c.api.GetPayment(ctx, &pb.GetPaymentRequest{
			PaymentId: req.PaymentID.String(),
		})

		if err != nil {
			return err
		}

		paymentID, err := uuid.Parse(resp.GetPaymentId())
		if err != nil {
			return err
		}

		t, err := time.Parse(time.RFC3339, resp.GetCreatedAt())
		if err != nil {
			return err
		}

		result = &payment.Payment{
			ID:        paymentID,
			Status:    protoconv.StatusFromProto(resp.GetStatus()),
			CreatedAt: t,
		}

		return nil
	})

	if err != nil {
		if err == circuitbreaker.ErrOpenState {
			return nil, fmt.Errorf("payment service temporarily unavailable (circuit breaker open): %w", err)
		}
		if err == circuitbreaker.ErrTooManyRequests {
			return nil, fmt.Errorf("payment service overloaded (too many requests): %w", err)
		}
		return nil, err
	}

	return result, nil
}

func (c *Client) CircuitBreakerStats() (circuitbreaker.State, circuitbreaker.Counts) {
	return c.cb.State(), c.cb.Counts()
}

func (c *Client) CircuitBreakerName() string {
	return c.cb.Name()
}

func isGRPCCallSuccessful(err error) bool {
	if err == nil {
		return true
	}

	if grpcStatus, ok := status.FromError(err); ok {
		switch grpcStatus.Code() {
		case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists,
			codes.PermissionDenied, codes.Unauthenticated:
			// Business-logic errors: server is healthy, request was just invalid.
			return true
		case codes.ResourceExhausted:
			// Server is alive and actively throttling — not a failure of the
			// downstream service. Do not penalise the circuit breaker counter.
			return true
		case codes.DeadlineExceeded, codes.Unavailable, codes.Internal, codes.Aborted:
			// Real infrastructure failures that warrant circuit breaker action.
			return false
		default:
			return false
		}
	}

	return false
}
