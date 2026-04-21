package payment

import "github.com/google/uuid"

type GetPaymentRequest struct {
	PaymentID uuid.UUID
}
