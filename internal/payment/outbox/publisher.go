package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/messaging"
	"github.com/google/uuid"
)

const (
	exchange   = "payments.events"
	maxRetries = 3
	batchSize  = 50
)

type outboxRow struct {
	ID          uuid.UUID
	AggregateID uuid.UUID
	EventType   string
	Payload     json.RawMessage
	Attempts    int
}

type Publisher struct {
	db       *sql.DB
	rabbit   *messaging.Publisher
	log      logger.Logger
	interval time.Duration
}

func NewPublisher(db *sql.DB, rabbit *messaging.Publisher, log logger.Logger) *Publisher {
	return &Publisher{
		db:       db,
		rabbit:   rabbit,
		log:      log,
		interval: 1 * time.Second,
	}
}

// Run inicia o polling loop. Bloqueia até ctx ser cancelado.
func (p *Publisher) Run(ctx context.Context) {
	p.log.Info(ctx, "outbox publisher started", "interval", p.interval.String(), "exchange", exchange)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.log.Info(ctx, "outbox publisher stopped")
			return
		case <-ticker.C:
			if err := p.poll(ctx); err != nil {
				p.log.Error(ctx, "outbox poll error", "err", err)
			}
		}
	}
}

func (p *Publisher) poll(ctx context.Context) error {
	const query = `
		SELECT id, aggregate_id, event_type, payload, attempts
		FROM outbox
		WHERE status = 'PENDING'
		ORDER BY created_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED`

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, query, batchSize)
	if err != nil {
		return err
	}
	defer rows.Close()

	var batch []outboxRow
	for rows.Next() {
		var r outboxRow
		if err := rows.Scan(&r.ID, &r.AggregateID, &r.EventType, &r.Payload, &r.Attempts); err != nil {
			return err
		}
		batch = append(batch, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(batch) == 0 {
		return tx.Commit()
	}

	for _, row := range batch {
		traceID, err := p.lookupTraceID(ctx, tx, row.AggregateID)
		if err != nil {
			p.log.Warn(ctx, "outbox trace_id lookup failed", "aggregate_id", row.AggregateID, "err", err)
			traceID = uuid.New().String()
		}

		var payload PaymentCreatedPayload
		_ = json.Unmarshal(row.Payload, &payload)

		headers := map[string]any{
			"x-trace-id": traceID,
		}
		if payload.Traceparent != "" {
			headers["traceparent"] = payload.Traceparent
		}
		if payload.Tracestate != "" {
			headers["tracestate"] = payload.Tracestate
		}

		if err := p.publishWithRetry(ctx, row, headers); err != nil {
			p.markFailed(ctx, tx, row)
			continue
		}

		p.markPublished(ctx, tx, row.ID)
	}

	return tx.Commit()
}

func (p *Publisher) lookupTraceID(ctx context.Context, tx *sql.Tx, aggregateID uuid.UUID) (string, error) {
	var traceID uuid.UUID
	err := tx.QueryRowContext(ctx,
		`SELECT trace_id FROM transactions WHERE id = $1`, aggregateID,
	).Scan(&traceID)
	if err != nil {
		return "", err
	}
	return traceID.String(), nil
}

func (p *Publisher) publishWithRetry(ctx context.Context, row outboxRow, headers map[string]interface{}) error {
	routingKey := row.EventType

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := p.rabbit.Publish(ctx, exchange, routingKey, row.Payload, headers); err != nil {
			lastErr = err
			p.log.Warn(ctx, "outbox publish attempt failed",
				"outbox_id", row.ID,
				"attempt", attempt,
				"err", err,
			)
			backoff := time.Duration(attempt*attempt) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			continue
		}
		return nil
	}

	p.log.Error(ctx, "outbox publish exhausted retries",
		"outbox_id", row.ID,
		"event_type", row.EventType,
		"err", lastErr,
	)
	return lastErr
}

func (p *Publisher) markPublished(ctx context.Context, tx *sql.Tx, id uuid.UUID) {
	const q = `UPDATE outbox SET status = 'PUBLISHED', published_at = NOW() WHERE id = $1`
	if _, err := tx.ExecContext(ctx, q, id); err != nil {
		p.log.Error(ctx, "outbox mark published failed", "outbox_id", id, "err", err)
	}
}

func (p *Publisher) markFailed(ctx context.Context, tx *sql.Tx, row outboxRow) {
	newAttempts := row.Attempts + 1
	if newAttempts >= maxRetries {
		const q = `UPDATE outbox SET status = 'FAILED', attempts = $2 WHERE id = $1`
		if _, err := tx.ExecContext(ctx, q, row.ID, newAttempts); err != nil {
			p.log.Error(ctx, "outbox mark failed error", "outbox_id", row.ID, "err", err)
		}
		return
	}
	const q = `UPDATE outbox SET attempts = $2 WHERE id = $1`
	if _, err := tx.ExecContext(ctx, q, row.ID, newAttempts); err != nil {
		p.log.Error(ctx, "outbox increment attempts failed", "outbox_id", row.ID, "err", err)
	}
}
