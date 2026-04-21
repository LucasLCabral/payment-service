package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	paymentoutbox "github.com/LucasLCabral/payment-service/internal/payment/outbox"
	"github.com/LucasLCabral/payment-service/pkg/payment"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type PaymentRepository struct {
	DB *sql.DB
}

func (r *PaymentRepository) FindByIdempotencyKey(ctx context.Context, tx *sql.Tx, idempotencyKey uuid.UUID) (*payment.Payment, error) {
	const q = `
		SELECT id, idempotency_key, amount_cents, currency, status,
		       payer_id, payee_id, description, decline_reason, trace_id,
		       created_at, updated_at
		FROM transactions WHERE idempotency_key = $1 FOR UPDATE`

	return scanPayment(tx.QueryRowContext(ctx, q, idempotencyKey))
}

func (r *PaymentRepository) Create(ctx context.Context, tx *sql.Tx, in *payment.CreatePaymentRequest, traceID uuid.UUID) (*payment.Payment, error) {
	const insertTx = `
		INSERT INTO transactions (
			idempotency_key, amount_cents, currency, status,
			payer_id, payee_id, description, trace_id
		) VALUES ($1, $2, $3, 'PENDING', $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`

	var descArg any
	if in.Description != "" {
		descArg = in.Description
	}

	var paymentID uuid.UUID
	var created, updated time.Time
	if err := tx.QueryRowContext(ctx, insertTx,
		in.IdempotencyKey, in.AmountCents, string(in.Currency),
		in.PayerID, in.PayeeID, descArg, traceID,
	).Scan(&paymentID, &created, &updated); err != nil {
		return nil, err
	}

	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	op := paymentoutbox.PaymentCreatedPayload{
		Event:          "payment.created",
		PaymentID:      paymentID.String(),
		IdempotencyKey: in.IdempotencyKey.String(),
		AmountCents:    in.AmountCents,
		Currency:       string(in.Currency),
		PayerID:        in.PayerID.String(),
		PayeeID:        in.PayeeID.String(),
		Traceparent:    carrier["traceparent"],
		Tracestate:     carrier["tracestate"],
	}
	payload, err := json.Marshal(op)
	if err != nil {
		return nil, err
	}

	const insertOutbox = `
		INSERT INTO outbox (aggregate_id, aggregate_type, event_type, payload, status)
		VALUES ($1, 'payment', 'payment.created', $2::jsonb, 'PENDING')`
	if _, err := tx.ExecContext(ctx, insertOutbox, paymentID, payload); err != nil {
		return nil, err
	}

	return &payment.Payment{
		ID:             paymentID,
		IdempotencyKey: in.IdempotencyKey,
		AmountCents:    in.AmountCents,
		Currency:       in.Currency,
		Status:         payment.StatusPending,
		PayerID:        in.PayerID,
		PayeeID:        in.PayeeID,
		Description:    in.Description,
		TraceID:        traceID,
		CreatedAt:      created.UTC(),
		UpdatedAt:      updated.UTC(),
	}, nil
}

func (r *PaymentRepository) FindByID(ctx context.Context, id uuid.UUID) (*payment.Payment, error) {
	const q = `
		SELECT id, idempotency_key, amount_cents, currency, status,
		       payer_id, payee_id, description, decline_reason, trace_id,
		       created_at, updated_at
		FROM transactions WHERE id = $1`

	return scanPayment(r.DB.QueryRowContext(ctx, q, id))
}

func (r *PaymentRepository) UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status payment.PaymentStatus, declineReason string) error {
	const q = `
		UPDATE transactions
		SET status = $1, decline_reason = $2, updated_at = NOW()
		WHERE id = $3 AND status = 'PENDING'`

	var reasonArg any
	if declineReason != "" {
		reasonArg = declineReason
	}

	res, err := tx.ExecContext(ctx, q, string(status), reasonArg, id)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *PaymentRepository) InsertAuditLog(ctx context.Context, tx *sql.Tx, e *payment.AuditEntry) error {
	const q = `
		INSERT INTO audit_log (entity_type, entity_id, action, old_status, new_status, trace_id, actor)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := tx.ExecContext(ctx, q,
		e.EntityType, e.EntityID, e.Action,
		string(e.OldStatus), string(e.NewStatus), e.TraceID, e.Actor,
	)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanPayment(row scanner) (*payment.Payment, error) {
	p := &payment.Payment{}
	var desc, decline sql.NullString
	err := row.Scan(
		&p.ID, &p.IdempotencyKey, &p.AmountCents, &p.Currency, &p.Status,
		&p.PayerID, &p.PayeeID, &desc, &decline, &p.TraceID,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	p.Description = desc.String
	p.DeclineReason = decline.String
	p.CreatedAt = p.CreatedAt.UTC()
	p.UpdatedAt = p.UpdatedAt.UTC()
	return p, nil
}
