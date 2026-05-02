package messaging

import (
	"context"
	"fmt"
	"time"

	"github.com/LucasLCabral/payment-service/pkg/circuitbreaker"
	amqp "github.com/rabbitmq/amqp091-go"
)

// PublisherWithCircuitBreaker wraps a RabbitMQ publisher with circuit breaker protection
type PublisherWithCircuitBreaker struct {
	publisher *Publisher
	cb        circuitbreaker.CircuitBreaker
}

// NewPublisherWithCircuitBreaker creates a RabbitMQ publisher wrapper with circuit breaker
func NewPublisherWithCircuitBreaker(cfg Config, serviceName string) (*PublisherWithCircuitBreaker, error) {
	publisher, err := NewPublisher(cfg)
	if err != nil {
		return nil, err
	}

	config := circuitbreaker.MessagingConfig()
	
	// Customize for RabbitMQ publishing
	config.MaxRequests = 3
	config.Timeout = 60 * time.Second
	config.ReadyToTrip = func(counts circuitbreaker.Counts) bool {
		// Message brokers can have temporary issues
		return counts.ConsecutiveFailures >= 4 || 
		       (counts.Requests >= 8 && float64(counts.TotalFailures)/float64(counts.Requests) >= 0.6)
	}
	
	cb := circuitbreaker.NewCircuitBreaker(
		fmt.Sprintf("rabbitmq-publisher-%s", serviceName),
		config,
	)

	return &PublisherWithCircuitBreaker{
		publisher: publisher,
		cb:        cb,
	}, nil
}

// PublishWithContext publishes a message with circuit breaker protection
func (p *PublisherWithCircuitBreaker) PublishWithContext(ctx context.Context, exchange, routingKey string, body []byte, headers map[string]interface{}) error {
	err := p.cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		return p.publisher.PublishWithContext(ctx, exchange, routingKey, body, headers)
	})
	
	if err != nil {
		return p.wrapError(err)
	}
	
	return nil
}

// Publish publishes a message with circuit breaker protection (backward compatibility)
func (p *PublisherWithCircuitBreaker) Publish(exchange, routingKey string, body []byte, headers map[string]interface{}) error {
	return p.PublishWithContext(context.Background(), exchange, routingKey, body, headers)
}

// Channel returns the underlying AMQP channel (use with caution)
func (p *PublisherWithCircuitBreaker) Channel() *amqp.Channel {
	return p.publisher.Channel()
}

// Close closes the publisher connection
func (p *PublisherWithCircuitBreaker) Close() error {
	return p.publisher.Close()
}

// CircuitBreakerStats returns the current state and statistics of the circuit breaker
func (p *PublisherWithCircuitBreaker) CircuitBreakerStats() (circuitbreaker.State, circuitbreaker.Counts) {
	return p.cb.State(), p.cb.Counts()
}

// CircuitBreakerName returns the name of the circuit breaker
func (p *PublisherWithCircuitBreaker) CircuitBreakerName() string {
	return p.cb.Name()
}

func (p *PublisherWithCircuitBreaker) wrapError(err error) error {
	if err == circuitbreaker.ErrOpenState {
		return fmt.Errorf("message broker temporarily unavailable (circuit breaker open): %w", err)
	}
	if err == circuitbreaker.ErrTooManyRequests {
		return fmt.Errorf("message broker overloaded (too many requests): %w", err)
	}
	return err
}

// Interface compliance check
var _ CircuitBreakerProvider = (*PublisherWithCircuitBreaker)(nil)

// CircuitBreakerProvider interface for RabbitMQ circuit breaker monitoring
type CircuitBreakerProvider interface {
	CircuitBreakerStats() (circuitbreaker.State, circuitbreaker.Counts)
	CircuitBreakerName() string
}