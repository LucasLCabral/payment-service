package circuitbreaker

import (
	"context"
	"fmt"
)

type Decorator struct {
	cb CircuitBreaker
}

func NewDecorator(name string, config Config) *Decorator {
	return &Decorator{
		cb: NewCircuitBreaker(name, config),
	}
}

func (d *Decorator) Execute(fn func() error) error {
	return d.cb.Execute(fn)
}

func (d *Decorator) ExecuteWithContext(ctx context.Context, fn func(ctx context.Context) error) error {
	err := d.cb.ExecuteWithContext(ctx, fn)
	return d.handleCircuitBreakerError(err)
}

func (d *Decorator) Stats() (State, Counts) {
	return d.cb.State(), d.cb.Counts()
}

func (d *Decorator) Name() string {
	return d.cb.Name()
}

// handleCircuitBreakerError converts circuit breaker errors to user-friendly messages
func (d *Decorator) handleCircuitBreakerError(err error) error {
	if err == nil {
		return nil
	}

	switch err {
	case ErrOpenState:
		return fmt.Errorf("service temporarily unavailable (circuit breaker open): %w", err)
	case ErrTooManyRequests:
		return fmt.Errorf("service overloaded (too many requests): %w", err)
	default:
		return err
	}
}
