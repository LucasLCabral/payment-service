package ledger

import "github.com/google/uuid"

// PaymentCreatedEvent is the payload received from the payment.created queue.
type PaymentCreatedEvent struct {
	Event          string    `json:"event"`
	PaymentID      uuid.UUID `json:"payment_id"`
	IdempotencyKey uuid.UUID `json:"idempotency_key"`
	AmountCents    int64     `json:"amount_cents"`
	Currency       string    `json:"currency"`
	PayerID        uuid.UUID `json:"payer_id"`
	PayeeID        uuid.UUID `json:"payee_id"`
}

// SettlementResult is the payload published in ledger.settled.accepted / .declined.
type SettlementResult struct {
	PaymentID     uuid.UUID `json:"payment_id"`
	Status        string    `json:"status"`
	DeclineReason string    `json:"decline_reason,omitempty"`
}
