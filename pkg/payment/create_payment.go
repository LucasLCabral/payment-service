package payment

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

func ParseCreatePaymentRequest(
	idempotencyKey string,
	amountCents int64,
	currency Currency,
	payerID string,
	payeeID string,
	description string,
) (*CreatePaymentRequest, error) {
	idem, err := uuid.Parse(idempotencyKey)
	if err != nil {
		return nil, ErrInvalidIdempotencyKey
	}
	payer, err := uuid.Parse(payerID)
	if err != nil {
		return nil, ErrInvalidPayerID
	}
	payee, err := uuid.Parse(payeeID)
	if err != nil {
		return nil, ErrInvalidPayeeID
	}
	return &CreatePaymentRequest{
		IdempotencyKey: idem,
		AmountCents:    amountCents,
		Currency:       currency,
		PayerID:        payer,
		PayeeID:        payee,
		Description:    description,
	}, nil
}
