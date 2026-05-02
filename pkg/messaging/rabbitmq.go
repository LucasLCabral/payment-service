package messaging

import (
	"context"
	"fmt"
	"time"

	"github.com/LucasLCabral/payment-service/pkg/circuitbreaker"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Config struct {
	URL string
}

type Publisher struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	cb      circuitbreaker.CircuitBreaker
}

func NewPublisher(ctx context.Context, cfg Config) (*Publisher, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	cb := circuitbreaker.NewCircuitBreaker("rabbitmq-publisher", circuitbreaker.MessagingConfig())

	return &Publisher{
		conn:    conn,
		channel: channel,
		cb:      cb,
	}, nil
}

func (p *Publisher) Publish(ctx context.Context, exchange, routingKey string, body []byte, headers map[string]interface{}) error {
	return p.cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		return p.channel.PublishWithContext(
			ctx,
			exchange,
			routingKey,
			false, // mandatory
			false, // immediate
			amqp.Publishing{
				ContentType:  "application/json",
				Body:         body,
				DeliveryMode: amqp.Persistent,
				Headers:      headers,
			},
		)
	})
}

func (p *Publisher) Channel() *amqp.Channel {
	return p.channel
}

func (p *Publisher) Close() error {
	if err := p.channel.Close(); err != nil {
		return err
	}
	return p.conn.Close()
}

func (p *Publisher) CircuitBreakerStats() (circuitbreaker.State, circuitbreaker.Counts) {
	return p.cb.State(), p.cb.Counts()
}

func (p *Publisher) CircuitBreakerName() string {
	return p.cb.Name()
}

type Consumer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
}

func NewConsumer(ctx context.Context, cfg Config) (*Consumer, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	return &Consumer{
		conn:    conn,
		channel: channel,
	}, nil
}

func (c *Consumer) Consume(ctx context.Context, queue string, handler func(ctx context.Context, msg amqp.Delivery) error) error {
	msgs, err := c.channel.Consume(
		queue,
		"",    // consumer tag
		false, // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("message channel closed")
			}

			if err := handler(ctx, msg); err != nil {
				requeue := !IsPermanent(err)
				if nackErr := msg.Nack(false, requeue); nackErr != nil {
					return fmt.Errorf("nack after handler error: %w (handler err: %v)", nackErr, err)
				}
			} else {
				if ackErr := msg.Ack(false); ackErr != nil {
					return fmt.Errorf("ack: %w", ackErr)
				}
			}
		}
	}
}

func RunConsumer(ctx context.Context, cfg Config, queue string, log logger.Logger, handler func(context.Context, amqp.Delivery) error) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		c, err := NewConsumer(ctx, cfg)
		if err != nil {
			log.Warn(ctx, "rabbitmq consumer connect failed", "queue", queue, "err", err)
			sleepCtx(ctx, backoff)
			backoff = minDur(backoff*2, maxBackoff)
			continue
		}
		backoff = time.Second
		log.Info(ctx, "rabbitmq consumer connected", "queue", queue)
		err = c.Consume(ctx, queue, handler)
		_ = c.Close()
		if ctx.Err() != nil {
			return
		}
		log.Warn(ctx, "rabbitmq consumer session ended", "queue", queue, "err", err)
		sleepCtx(ctx, backoff)
		backoff = minDur(backoff*2, maxBackoff)
	}
}

func (c *Consumer) Close() error {
	if err := c.channel.Close(); err != nil {
		return err
	}
	return c.conn.Close()
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func minDur(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
