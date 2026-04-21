package models

type PaymentStatus string

const (
	PaymentStatusPending     PaymentStatus = "PENDING"
	PaymentStatusSettled     PaymentStatus = "SETTLED"
	PaymentStatusDeclined    PaymentStatus = "DECLINED"
	PaymentStatusFailed      PaymentStatus = "FAILED"
	PaymentStatusUnspecified PaymentStatus = "UNSPECIFIED"
)

type Currency string

const (
	CurrencyBRL Currency = "BRL"
	CurrencyUSD Currency = "USD"
)

func (c Currency) IsValid() bool {
	return c == CurrencyBRL || c == CurrencyUSD
}
