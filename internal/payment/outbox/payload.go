package outbox

type PaymentCreatedPayload struct {
	Event          string `json:"event"`
	PaymentID      string `json:"payment_id"`
	IdempotencyKey string `json:"idempotency_key"`
	AmountCents    int64  `json:"amount_cents"`
	Currency       string `json:"currency"`
	PayerID        string `json:"payer_id"`
	PayeeID        string `json:"payee_id"`
	Traceparent    string `json:"traceparent,omitempty"`
	Tracestate     string `json:"tracestate,omitempty"`
}
