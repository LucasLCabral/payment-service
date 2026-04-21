package repository

import (
	"context"
	"database/sql"

	"github.com/LucasLCabral/payment-service/pkg/payment"
	"github.com/google/uuid"
)

type Payment interface {
	GetByIdempotencyKeyForUpdate(ctx context.Context, tx *sql.Tx, idempotencyKey uuid.UUID) (*payment.Payment, error)
	InsertPaymentWithOutbox(ctx context.Context, tx *sql.Tx, in *payment.CreatePaymentRequest, traceID uuid.UUID) (*payment.Payment, error)
	GetByID(ctx context.Context, id uuid.UUID) (*payment.Payment, error)
}
