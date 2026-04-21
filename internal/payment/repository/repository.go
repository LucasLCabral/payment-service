package repository

import (
	"context"
	"database/sql"

	model "github.com/LucasLCabral/payment-service/internal/payment/models"
	"github.com/google/uuid"
)

type Payment interface {
	GetByIdempotencyKeyForUpdate(ctx context.Context, tx *sql.Tx, idempotencyKey uuid.UUID) (*model.Payment, error)
	InsertPaymentWithOutbox(ctx context.Context, tx *sql.Tx, req *model.CreatePaymentRequest, traceID uuid.UUID) (*model.Payment, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Payment, error)
}
