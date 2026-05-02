package notify

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/LucasLCabral/payment-service/pkg/circuitbreaker"
	"github.com/LucasLCabral/payment-service/pkg/payment"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type StatusPayload struct {
	PaymentID     string `json:"payment_id"`
	Status        string `json:"status"`
	DeclineReason string `json:"decline_reason,omitempty"`
}

type RedisPublisher struct {
	rdb *redis.Client
	cb  circuitbreaker.CircuitBreaker
}

const ChannelPaymentStatus = "payment:status"

func ConnectRedis(ctx context.Context, addrOrURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(addrOrURL)
	if err != nil {
		opts = &redis.Options{Addr: addrOrURL}
	}
	rdb := redis.NewClient(opts)
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return rdb, nil
}

func NewRedisPublisher(rdb *redis.Client) *RedisPublisher {
	config := circuitbreaker.DefaultConfig()
	config.MaxRequests = 5
	config.ReadyToTrip = func(counts circuitbreaker.Counts) bool {
		return counts.ConsecutiveFailures >= 5 ||
			(counts.Requests >= 10 && float64(counts.TotalFailures)/float64(counts.Requests) >= 0.7)
	}

	cb := circuitbreaker.NewCircuitBreaker("redis-publisher", config)

	return &RedisPublisher{
		rdb: rdb,
		cb:  cb,
	}
}

func (p *RedisPublisher) NotifyPaymentStatus(ctx context.Context, paymentID uuid.UUID, status payment.PaymentStatus, declineReason string) error {
	if p == nil || p.rdb == nil {
		return nil
	}

	return p.cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		body, err := json.Marshal(StatusPayload{
			PaymentID:     paymentID.String(),
			Status:        string(status),
			DeclineReason: declineReason,
		})
		if err != nil {
			return err
		}
		return p.rdb.Publish(ctx, ChannelPaymentStatus, body).Err()
	})
}

func (p *RedisPublisher) CircuitBreakerStats() (circuitbreaker.State, circuitbreaker.Counts) {
	return p.cb.State(), p.cb.Counts()
}

func (p *RedisPublisher) CircuitBreakerName() string {
	return p.cb.Name()
}
