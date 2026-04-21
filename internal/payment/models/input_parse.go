package models

import (
	"github.com/google/uuid"
)

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
