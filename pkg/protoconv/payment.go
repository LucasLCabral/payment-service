package protoconv

import (
	"github.com/LucasLCabral/payment-service/pkg/payment"
	"github.com/LucasLCabral/payment-service/protog/common"
)

func CurrencyToProto(c payment.Currency) common.Currency {
	switch c {
	case payment.CurrencyBRL:
		return common.Currency_CURRENCY_BRL
	case payment.CurrencyUSD:
		return common.Currency_CURRENCY_USD
	default:
		return common.Currency_CURRENCY_UNSPECIFIED
	}
}

func CurrencyFromProto(c common.Currency) (payment.Currency, bool) {
	switch c {
	case common.Currency_CURRENCY_BRL:
		return payment.CurrencyBRL, true
	case common.Currency_CURRENCY_USD:
		return payment.CurrencyUSD, true
	default:
		return "", false
	}
}

func CurrencyFromProtoUnsafe(c common.Currency) payment.Currency {
	cur, _ := CurrencyFromProto(c)
	return cur
}

func StatusToProto(s payment.PaymentStatus) common.PaymentStatus {
	switch s {
	case payment.StatusPending:
		return common.PaymentStatus_PAYMENT_STATUS_PENDING
	case payment.StatusSettled:
		return common.PaymentStatus_PAYMENT_STATUS_SETTLED
	case payment.StatusDeclined:
		return common.PaymentStatus_PAYMENT_STATUS_DECLINED
	case payment.StatusFailed:
		return common.PaymentStatus_PAYMENT_STATUS_FAILED
	default:
		return common.PaymentStatus_PAYMENT_STATUS_UNSPECIFIED
	}
}

func StatusFromProto(s common.PaymentStatus) payment.PaymentStatus {
	switch s {
	case common.PaymentStatus_PAYMENT_STATUS_PENDING:
		return payment.StatusPending
	case common.PaymentStatus_PAYMENT_STATUS_SETTLED:
		return payment.StatusSettled
	case common.PaymentStatus_PAYMENT_STATUS_DECLINED:
		return payment.StatusDeclined
	case common.PaymentStatus_PAYMENT_STATUS_FAILED:
		return payment.StatusFailed
	default:
		return payment.StatusUnspecified
	}
}
