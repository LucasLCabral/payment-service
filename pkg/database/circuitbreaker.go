package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LucasLCabral/payment-service/pkg/circuitbreaker"
)

type CBDatabase struct {
	db *sql.DB
	cb circuitbreaker.CircuitBreaker
}

func NewCBDatabase(db *sql.DB, serviceName string) *CBDatabase {
	config := circuitbreaker.DatabaseConfig()
	config.MaxRequests = 5
	config.ReadyToTrip = func(counts circuitbreaker.Counts) bool {
		return counts.ConsecutiveFailures >= 3 ||
			(counts.Requests >= 10 && float64(counts.TotalFailures)/float64(counts.Requests) >= 0.5)
	}

	cb := circuitbreaker.NewCircuitBreaker(
		fmt.Sprintf("database-%s", serviceName),
		config,
	)

	return &CBDatabase{
		db: db,
		cb: cb,
	}
}

func (cbd *CBDatabase) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	var tx *sql.Tx
	err := cbd.cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		var err error
		tx, err = cbd.db.BeginTx(ctx, opts)
		return err
	})
	return tx, err
}

func (cbd *CBDatabase) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	var row *sql.Row
	_ = cbd.cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		row = cbd.db.QueryRowContext(ctx, query, args...)
		return nil
	})
	return row
}

func (cbd *CBDatabase) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	var rows *sql.Rows
	err := cbd.cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		var err error
		rows, err = cbd.db.QueryContext(ctx, query, args...)
		return err
	})
	return rows, err
}

func (cbd *CBDatabase) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	var result sql.Result
	err := cbd.cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		var err error
		result, err = cbd.db.ExecContext(ctx, query, args...)
		return err
	})
	return result, err
}

func (cbd *CBDatabase) Close() error {
	return cbd.db.Close()
}

func (cbd *CBDatabase) PingContext(ctx context.Context) error {
	return cbd.cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		return cbd.db.PingContext(ctx)
	})
}

func (cbd *CBDatabase) CircuitBreakerStats() (circuitbreaker.State, circuitbreaker.Counts) {
	return cbd.cb.State(), cbd.cb.Counts()
}

func (cbd *CBDatabase) CircuitBreakerName() string {
	return cbd.cb.Name()
}

func WithCBTransaction(ctx context.Context, cbd *CBDatabase, fn func(tx *sql.Tx) error) error {
	return cbd.cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		return WithTransaction(ctx, cbd.db, fn)
	})
}