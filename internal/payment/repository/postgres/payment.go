package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	model "github.com/LucasLCabral/payment-service/internal/payment/models"
	"github.com/google/uuid"
)

type outboxPayload struct {
	Event     string `json:"event"`
	PaymentID string `json:"payment_id"`
}

type PaymentRepository struct {
	DB *sql.DB
}

func (r *PaymentRepository) GetByIdempotencyKeyForUpdate(ctx context.Context, tx *sql.Tx, idempotencyKey uuid.UUID) (*model.Payment, error) {
	const q = `
		SELECT id, idempotency_key, amount_cents, currency, status,
		       payer_id, payee_id, description, decline_reason, trace_id,
		       created_at, updated_at
		FROM transactions WHERE idempotency_key = $1 FOR UPDATE`

	p := &model.Payment{}
	var desc, decline sql.NullString
	err := tx.QueryRowContext(ctx, q, idempotencyKey).Scan(
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

func (r *PaymentRepository) InsertPaymentWithOutbox(ctx context.Context, tx *sql.Tx, req *model.CreatePaymentRequest, traceID uuid.UUID) (*model.Payment, error) {
	const insertTx = `
		INSERT INTO transactions (
			idempotency_key, amount_cents, currency, status,
			payer_id, payee_id, description, trace_id
		) VALUES ($1, $2, $3, 'PENDING', $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`

	var descArg any
	if req.Description != "" {
		descArg = req.Description
	}

	var paymentID uuid.UUID
	var created, updated time.Time
	if err := tx.QueryRowContext(ctx, insertTx,
		req.IdempotencyKey, req.AmountCents, string(req.Currency),
		req.PayerID, req.PayeeID, descArg, traceID,
	).Scan(&paymentID, &created, &updated); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(outboxPayload{
		Event:     "payment.created",
		PaymentID: paymentID.String(),
	})
	if err != nil {
		return nil, err
	}

	const insertOutbox = `
		INSERT INTO outbox (aggregate_id, aggregate_type, event_type, payload, status)
		VALUES ($1, 'payment', 'payment.created', $2::jsonb, 'PENDING')`
	if _, err := tx.ExecContext(ctx, insertOutbox, paymentID, payload); err != nil {
		return nil, err
	}

	return &model.Payment{
		ID:             paymentID,
		IdempotencyKey: req.IdempotencyKey,
		AmountCents:    req.AmountCents,
		Currency:       req.Currency,
		Status:         model.PaymentStatusPending,
		PayerID:        req.PayerID,
		PayeeID:        req.PayeeID,
		Description:    req.Description,
		TraceID:        traceID,
		CreatedAt:      created.UTC(),
		UpdatedAt:      updated.UTC(),
	}, nil
}

func (r *PaymentRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Payment, error) {
	const q = `
		SELECT id, idempotency_key, amount_cents, currency, status,
		       payer_id, payee_id, description, decline_reason, trace_id,
		       created_at, updated_at
		FROM transactions WHERE id = $1`

	p := &model.Payment{}
	var desc, decline sql.NullString
	err := r.DB.QueryRowContext(ctx, q, id).Scan(
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
