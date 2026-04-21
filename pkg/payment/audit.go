package payment

import "github.com/google/uuid"

type AuditEntry struct {
	EntityType string
	EntityID   uuid.UUID
	Action     string
	OldStatus  PaymentStatus
	NewStatus  PaymentStatus
	TraceID    uuid.UUID
	Actor      string
}
