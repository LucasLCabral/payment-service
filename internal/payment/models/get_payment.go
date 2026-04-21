package models

import "github.com/google/uuid"

type GetPaymentRequest struct {
	PaymentID uuid.UUID
}
