package ledger

import (
	"time"

	"github.com/google/uuid"
)

type Entry struct {
	ID             uuid.UUID
	PaymentID      uuid.UUID
	IdempotencyKey uuid.UUID
	AmountCents    int64
	Currency       string
	Direction      Direction
	PayerID        uuid.UUID
	PayeeID        uuid.UUID
	Status         EntryStatus
	DeclineReason  string
	TraceID        uuid.UUID
	CreatedAt      time.Time
}
