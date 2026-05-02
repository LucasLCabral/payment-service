package notify

import (
	"context"

	"github.com/LucasLCabral/payment-service/pkg/circuitbreaker"
	"github.com/redis/go-redis/v9"
)

type RedisSubscriber struct {
	rdb *redis.Client
	cb  circuitbreaker.CircuitBreaker
}

func NewRedisSubscriber(rdb *redis.Client) *RedisSubscriber {
	config := circuitbreaker.DefaultConfig()
	config.MaxRequests = 3
	config.ReadyToTrip = func(counts circuitbreaker.Counts) bool {
		return counts.ConsecutiveFailures >= 3 ||
			(counts.Requests >= 8 && float64(counts.TotalFailures)/float64(counts.Requests) >= 0.6)
	}

	cb := circuitbreaker.NewCircuitBreaker("redis-subscriber", config)

	return &RedisSubscriber{
		rdb: rdb,
		cb:  cb,
	}
}

func (s *RedisSubscriber) Subscribe(ctx context.Context, channels ...string) (*redis.PubSub, error) {
	var pubSub *redis.PubSub
	err := s.cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		pubSub = s.rdb.Subscribe(ctx, channels...)
		return nil
	})
	return pubSub, err
}

func (s *RedisSubscriber) CircuitBreakerStats() (circuitbreaker.State, circuitbreaker.Counts) {
	return s.cb.State(), s.cb.Counts()
}

func (s *RedisSubscriber) CircuitBreakerName() string {
	return s.cb.Name()
}