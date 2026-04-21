package payment

import (
	"time"

	"github.com/google/uuid"
)

type Payment struct {
	ID             uuid.UUID
	IdempotencyKey uuid.UUID
	AmountCents    int64
	Currency       Currency
	Status         PaymentStatus
	PayerID        uuid.UUID
	PayeeID        uuid.UUID
	Description    string
	DeclineReason  string
	TraceID        uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
