package repository

import (
	"context"
	"database/sql"

	"github.com/LucasLCabral/payment-service/pkg/payment"
	"github.com/google/uuid"
)

type Payment interface {
	Create(ctx context.Context, tx *sql.Tx, in *payment.CreatePaymentRequest, traceID uuid.UUID) (*payment.Payment, error)
	FindByID(ctx context.Context, id uuid.UUID) (*payment.Payment, error)
	FindByIdempotencyKey(ctx context.Context, tx *sql.Tx, key uuid.UUID) (*payment.Payment, error)
	UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status payment.PaymentStatus, declineReason string) error
}

type Audit interface {
	Insert(ctx context.Context, tx *sql.Tx, entry *payment.AuditEntry) error
}
