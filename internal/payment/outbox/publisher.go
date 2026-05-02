package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"time"

	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/messaging"
	"github.com/google/uuid"
)

const (
	exchange  = "payments.events"
	batchSize = 100

	// maxAttempts is the hard cap before a message becomes FAILED.
	maxAttempts = 5

	// baseBackoff is the seed for exponential retry scheduling.
	// attempt 1 → 4s, 2 → 8s, 3 → 16s, 4 → 32s, 5 → FAILED
	baseBackoff = 2 * time.Second

	// idleWait is how long to sleep when there is nothing ready to process.
	idleWait = 1 * time.Second

	// stuckThreshold is how long a row can sit in PROCESSING before the
	// watchdog considers it orphaned and resets it to PENDING.
	stuckThreshold = 5 * time.Minute
)

type outboxRow struct {
	ID          uuid.UUID
	AggregateID uuid.UUID
	EventType   string
	Payload     json.RawMessage
	Attempts    int
}

// Publisher polls the outbox table and forwards events to RabbitMQ.
//
// Status lifecycle:
//
//	PENDING ──► PROCESSING ──► PUBLISHED
//	                │
//	                └──► PENDING  (watchdog recovery after stuckThreshold)
//	                └──► FAILED   (after maxAttempts, scheduleRetry)
//
// The two-transaction design means a pod crash between "mark PROCESSING" and
// "publish" leaves rows in PROCESSING. The watchdog detects them by
// processing_since and resets them to PENDING so another pod retries.
type Publisher struct {
	db     *sql.DB
	rabbit *messaging.Publisher
	log    logger.Logger
}

func NewPublisher(db *sql.DB, rabbit *messaging.Publisher, log logger.Logger) *Publisher {
	return &Publisher{db: db, rabbit: rabbit, log: log}
}

// Run polls the outbox and runs the watchdog until ctx is cancelled.
func (p *Publisher) Run(ctx context.Context) {
	p.log.Info(ctx, "outbox publisher started", "exchange", exchange,
		"batch_size", batchSize, "max_attempts", maxAttempts)

	go p.runWatchdog(ctx)

	for {
		n, err := p.poll(ctx)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			p.log.Error(ctx, "outbox poll error", "err", err)
		}

		// Full batch means there is likely more work — skip idle sleep.
		if n == batchSize {
			continue
		}

		select {
		case <-ctx.Done():
		case <-time.After(idleWait):
		}

		if ctx.Err() != nil {
			break
		}
	}

	p.log.Info(ctx, "outbox publisher stopped")
}

// poll processes one batch. Returns the number of rows handled.
func (p *Publisher) poll(ctx context.Context) (int, error) {
	batch, err := p.claimBatch(ctx)
	if err != nil || len(batch) == 0 {
		return 0, err
	}

	for _, row := range batch {
		if err := p.publish(ctx, row); err != nil {
			p.scheduleRetry(ctx, row)
			continue
		}
		p.markPublished(ctx, row.ID)
	}

	return len(batch), nil
}

// claimBatch atomically moves a batch of PENDING rows to PROCESSING.
// Using a dedicated transaction here means the lock is released immediately
// after the status update — other publishers can claim their own batches
// without waiting for RabbitMQ round trips.
func (p *Publisher) claimBatch(ctx context.Context) ([]outboxRow, error) {
	const query = `
		UPDATE outbox
		SET status = 'PROCESSING', processing_since = NOW()
		WHERE id IN (
			SELECT id FROM outbox
			WHERE status = 'PENDING'
			  AND next_retry_at <= NOW()
			ORDER BY next_retry_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, aggregate_id, event_type, payload, attempts`

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.QueryContext(ctx, query, batchSize)
	if err != nil {
		return nil, err
	}

	var batch []outboxRow
	for rows.Next() {
		var r outboxRow
		if err := rows.Scan(&r.ID, &r.AggregateID, &r.EventType, &r.Payload, &r.Attempts); err != nil {
			rows.Close()
			return nil, err
		}
		batch = append(batch, r)
	}
	rows.Close()

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return batch, tx.Commit()
}

// publish sends a single event. Does not retry — retry scheduling belongs to
// the poll loop via scheduleRetry.
func (p *Publisher) publish(ctx context.Context, row outboxRow) error {
	headers := extractHeaders(row.Payload)

	if err := p.rabbit.Publish(ctx, exchange, row.EventType, row.Payload, headers); err != nil {
		p.log.Warn(ctx, "outbox publish failed",
			"outbox_id", row.ID,
			"event_type", row.EventType,
			"attempts", row.Attempts+1,
			"err", err,
		)
		return err
	}
	return nil
}

// scheduleRetry computes next_retry_at using exponential backoff and marks
// the row FAILED once maxAttempts is reached.
//
// Formula: baseBackoff * 2^attempts
// → attempt 1 = 4s, 2 = 8s, 3 = 16s, 4 = 32s, 5 = FAILED
func (p *Publisher) scheduleRetry(ctx context.Context, row outboxRow) {
	newAttempts := row.Attempts + 1

	if newAttempts >= maxAttempts {
		const q = `
			UPDATE outbox
			SET status = 'FAILED', attempts = $2, processing_since = NULL
			WHERE id = $1`
		if _, err := p.db.ExecContext(ctx, q, row.ID, newAttempts); err != nil {
			p.log.Error(ctx, "outbox mark failed error", "outbox_id", row.ID, "err", err)
		}
		p.log.Error(ctx, "outbox message permanently failed",
			"outbox_id", row.ID, "event_type", row.EventType, "attempts", newAttempts)
		return
	}

	delay := time.Duration(math.Pow(2, float64(newAttempts))) * baseBackoff
	const q = `
		UPDATE outbox
		SET status = 'PENDING', attempts = $2, next_retry_at = NOW() + $3,
		    processing_since = NULL
		WHERE id = $1`
	if _, err := p.db.ExecContext(ctx, q, row.ID, newAttempts, delay); err != nil {
		p.log.Error(ctx, "outbox schedule retry failed", "outbox_id", row.ID, "err", err)
	}
}

func (p *Publisher) markPublished(ctx context.Context, id uuid.UUID) {
	const q = `
		UPDATE outbox
		SET status = 'PUBLISHED', published_at = NOW(), processing_since = NULL
		WHERE id = $1`
	if _, err := p.db.ExecContext(ctx, q, id); err != nil {
		p.log.Error(ctx, "outbox mark published failed", "outbox_id", id, "err", err)
	}
}

// runWatchdog periodically resets rows stuck in PROCESSING back to PENDING.
// This recovers from pod crashes that happened between claimBatch and markPublished.
func (p *Publisher) runWatchdog(ctx context.Context) {
	// Run half as often as stuckThreshold to avoid thundering herd.
	ticker := time.NewTicker(stuckThreshold / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.recoverStuck(ctx)
		}
	}
}

// recoverStuck resets rows that have been in PROCESSING longer than stuckThreshold.
func (p *Publisher) recoverStuck(ctx context.Context) {
	const q = `
		UPDATE outbox
		SET status = 'PENDING', processing_since = NULL, next_retry_at = NOW()
		WHERE status = 'PROCESSING'
		  AND processing_since < NOW() - $1::interval`

	res, err := p.db.ExecContext(ctx, q, stuckThreshold.String())
	if err != nil {
		p.log.Error(ctx, "outbox watchdog failed", "err", err)
		return
	}

	if n, _ := res.RowsAffected(); n > 0 {
		p.log.Warn(ctx, "outbox watchdog recovered stuck messages",
			"count", n, "stuck_threshold", stuckThreshold)
	}
}

// extractHeaders reads W3C trace propagation fields from the raw JSON payload
// without deserialising the full domain type, keeping the publisher decoupled
// from event schemas.
func extractHeaders(raw json.RawMessage) map[string]any {
	var envelope struct {
		Traceparent string `json:"traceparent"`
		Tracestate  string `json:"tracestate"`
		TraceID     string `json:"trace_id"`
	}
	_ = json.Unmarshal(raw, &envelope) // safe: missing fields are empty strings

	headers := make(map[string]any, 3)
	if envelope.TraceID != "" {
		headers["x-trace-id"] = envelope.TraceID
	}
	if envelope.Traceparent != "" {
		headers["traceparent"] = envelope.Traceparent
	}
	if envelope.Tracestate != "" {
		headers["tracestate"] = envelope.Tracestate
	}
	return headers
}
