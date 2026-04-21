package models

import (
	"unicode/utf8"

	"github.com/google/uuid"
)

type CreatePaymentRequest struct {
	IdempotencyKey uuid.UUID
	AmountCents    int64
	Currency       Currency
	PayerID        uuid.UUID
	PayeeID        uuid.UUID
	Description    string
}

func (r *CreatePaymentRequest) Validate() error {
	if r == nil {
		return NewValidationError("input", "must not be nil")
	}
	if r.AmountCents <= 0 {
		return ErrAmountNotPositive
	}
	if !r.Currency.IsValid() {
		return ErrCurrencyRequired
	}
	if utf8.RuneCountInString(r.Description) > 255 {
		return ErrDescriptionTooLong
	}
	return nil
}
