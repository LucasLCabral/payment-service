package models

import (
	"errors"
	"fmt"
)


type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return e.Message
}

func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{Field: field, Message: message}
}

func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

var (
	ErrAmountNotPositive     = NewValidationError("amount_cents", "must be positive")
	ErrCurrencyRequired      = NewValidationError("currency", "is required")
	ErrInvalidIdempotencyKey = NewValidationError("idempotency_key", "must be a valid UUID")
	ErrInvalidPayerID        = NewValidationError("payer_id", "must be a valid UUID")
	ErrInvalidPayeeID        = NewValidationError("payee_id", "must be a valid UUID")
	ErrDescriptionTooLong    = NewValidationError("description", "max 255 characters")
)
