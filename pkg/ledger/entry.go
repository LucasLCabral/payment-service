package ledger

import (
	"time"

	"github.com/google/uuid"
)

type Entry struct {
	ID             uuid.UUID
	PaymentID      uuid.UUID
	IdempotencyKey uuid.UUID
	AccountID      uuid.UUID
	AmountCents    int64
	Currency       string
	Direction      Direction
	Status         EntryStatus
	DeclineReason  string
	TraceID        uuid.UUID
	CreatedAt      time.Time
}
