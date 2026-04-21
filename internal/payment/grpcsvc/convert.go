package grpcsvc

import (
	"time"

	"github.com/LucasLCabral/payment-service/pkg/payment"
	"github.com/LucasLCabral/payment-service/protog/common"
	pb "github.com/LucasLCabral/payment-service/protog/payment"
	"github.com/google/uuid"
)

func createRequestToModel(req *pb.CreatePaymentRequest) (*payment.CreatePaymentRequest, error) {
	cur, ok := currencyFromProto(req.GetCurrency())
	if !ok {
		return nil, payment.ErrCurrencyRequired
	}
	return payment.ParseCreatePaymentRequest(
		req.GetIdempotencyKey(),
		req.GetAmountCents(),
		cur,
		req.GetPayerId(),
		req.GetPayeeId(),
		req.GetDescription(),
	)
}

func paymentToCreateResponse(p *payment.Payment) *pb.CreatePaymentResponse {
	return &pb.CreatePaymentResponse{
		PaymentId: p.ID.String(),
		Status:    statusToProto(p.Status),
		CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func getRequestToModel(req *pb.GetPaymentRequest) (*payment.GetPaymentRequest, error) {
	pid, err := uuid.Parse(req.GetPaymentId())
	if err != nil {
		return nil, err
	}
	return &payment.GetPaymentRequest{PaymentID: pid}, nil
}

func paymentToGetResponse(p *payment.Payment) *pb.GetPaymentResponse {
	return &pb.GetPaymentResponse{
		PaymentId:     p.ID.String(),
		Status:        statusToProto(p.Status),
		AmountCents:   p.AmountCents,
		Currency:      currencyToProto(p.Currency),
		CreatedAt:     p.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     p.UpdatedAt.UTC().Format(time.RFC3339),
		DeclineReason: p.DeclineReason,
	}
}

func currencyToProto(c payment.Currency) common.Currency {
	switch c {
	case payment.CurrencyBRL:
		return common.Currency_CURRENCY_BRL
	case payment.CurrencyUSD:
		return common.Currency_CURRENCY_USD
	default:
		return common.Currency_CURRENCY_UNSPECIFIED
	}
}

func currencyFromProto(c common.Currency) (payment.Currency, bool) {
	switch c {
	case common.Currency_CURRENCY_BRL:
		return payment.CurrencyBRL, true
	case common.Currency_CURRENCY_USD:
		return payment.CurrencyUSD, true
	default:
		return "", false
	}
}

func statusFromProto(s common.PaymentStatus) payment.PaymentStatus {
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

func statusToProto(s payment.PaymentStatus) common.PaymentStatus {
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
