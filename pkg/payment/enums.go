package payment

type PaymentStatus string

const (
	StatusPending     PaymentStatus = "PENDING"
	StatusSettled     PaymentStatus = "SETTLED"
	StatusDeclined    PaymentStatus = "DECLINED"
	StatusFailed      PaymentStatus = "FAILED"
	StatusUnspecified PaymentStatus = "UNSPECIFIED"
)

type Currency string

const (
	CurrencyBRL Currency = "BRL"
	CurrencyUSD Currency = "USD"
)

func (c Currency) IsValid() bool {
	return c == CurrencyBRL || c == CurrencyUSD
}
