package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/LucasLCabral/payment-service/pkg/circuitbreaker"
)

// ConnectionWithCircuitBreaker wraps a database connection with circuit breaker protection
type ConnectionWithCircuitBreaker struct {
	db *sql.DB
	cb circuitbreaker.CircuitBreaker
}

// NewConnectionWithCircuitBreaker creates a database connection wrapper with circuit breaker
func NewConnectionWithCircuitBreaker(db *sql.DB, serviceName string) *ConnectionWithCircuitBreaker {
	config := circuitbreaker.DatabaseConfig()
	
	// Customize for database operations
	config.MaxRequests = 2
	config.Timeout = 90 * time.Second // DB recovery takes longer
	config.ReadyToTrip = func(counts circuitbreaker.Counts) bool {
		// Be conservative with database - it's critical infrastructure
		return counts.ConsecutiveFailures >= 5 || 
		       (counts.Requests >= 10 && float64(counts.TotalFailures)/float64(counts.Requests) >= 0.7)
	}
	
	cb := circuitbreaker.NewCircuitBreaker(
		fmt.Sprintf("database-%s", serviceName),
		config,
	)

	return &ConnectionWithCircuitBreaker{
		db: db,
		cb: cb,
	}
}

// Query executes a query with circuit breaker protection
func (c *ConnectionWithCircuitBreaker) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	var rows *sql.Rows
	
	err := c.cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		var err error
		rows, err = c.db.QueryContext(ctx, query, args...)
		return err
	})
	
	if err != nil {
		return nil, c.wrapError(err)
	}
	
	return rows, nil
}

// QueryRow executes a query that returns at most one row with circuit breaker protection
func (c *ConnectionWithCircuitBreaker) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	// For QueryRow, we need a different approach since it never returns an error
	// We'll use a custom wrapper that checks the circuit breaker state
	if c.cb.State() == circuitbreaker.StateOpen {
		// Return a row that will produce an error when scanned
		return &sql.Row{}
	}
	
	return c.db.QueryRowContext(ctx, query, args...)
}

// Exec executes a query without returning any rows with circuit breaker protection
func (c *ConnectionWithCircuitBreaker) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	var result sql.Result
	
	err := c.cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		var err error
		result, err = c.db.ExecContext(ctx, query, args...)
		return err
	})
	
	if err != nil {
		return nil, c.wrapError(err)
	}
	
	return result, nil
}

// BeginTx starts a transaction with circuit breaker protection
func (c *ConnectionWithCircuitBreaker) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	var tx *sql.Tx
	
	err := c.cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		var err error
		tx, err = c.db.BeginTx(ctx, opts)
		return err
	})
	
	if err != nil {
		return nil, c.wrapError(err)
	}
	
	return tx, nil
}

// Ping verifies the connection to the database with circuit breaker protection
func (c *ConnectionWithCircuitBreaker) Ping(ctx context.Context) error {
	err := c.cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		return c.db.PingContext(ctx)
	})
	
	if err != nil {
		return c.wrapError(err)
	}
	
	return nil
}

// Close closes the database connection
func (c *ConnectionWithCircuitBreaker) Close() error {
	return c.db.Close()
}

// Stats returns the connection statistics
func (c *ConnectionWithCircuitBreaker) Stats() sql.DBStats {
	return c.db.Stats()
}

// CircuitBreakerStats returns the current state and statistics of the circuit breaker
func (c *ConnectionWithCircuitBreaker) CircuitBreakerStats() (circuitbreaker.State, circuitbreaker.Counts) {
	return c.cb.State(), c.cb.Counts()
}

// CircuitBreakerName returns the name of the circuit breaker
func (c *ConnectionWithCircuitBreaker) CircuitBreakerName() string {
	return c.cb.Name()
}

// DB returns the underlying database connection (use with caution)
func (c *ConnectionWithCircuitBreaker) DB() *sql.DB {
	return c.db
}

func (c *ConnectionWithCircuitBreaker) wrapError(err error) error {
	if err == circuitbreaker.ErrOpenState {
		return fmt.Errorf("database temporarily unavailable (circuit breaker open): %w", err)
	}
	if err == circuitbreaker.ErrTooManyRequests {
		return fmt.Errorf("database overloaded (too many requests): %w", err)
	}
	return err
}

// Interface compliance check
var _ CircuitBreakerProvider = (*ConnectionWithCircuitBreaker)(nil)

// CircuitBreakerProvider interface for database circuit breaker monitoring
type CircuitBreakerProvider interface {
	CircuitBreakerStats() (circuitbreaker.State, circuitbreaker.Counts)
	CircuitBreakerName() string
}