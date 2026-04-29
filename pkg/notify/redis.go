package notify

import (
	"context"
	"fmt"

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
}

const StreamPaymentNotifications = "payment:notifications"
const ConsumerGroup = "api-gateway-group"

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
	return &RedisPublisher{rdb: rdb}
}

func (p *RedisPublisher) NotifyPaymentStatus(ctx context.Context, paymentID uuid.UUID, status payment.PaymentStatus, declineReason string) error {
	if p == nil || p.rdb == nil {
		return nil
	}
	
	return p.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: StreamPaymentNotifications,
		Values: map[string]interface{}{
			"payment_id":     paymentID.String(),
			"status":         string(status),
			"decline_reason": declineReason,
		},
	}).Err()
}
