package grpcsvc

import (
	"time"

	model "github.com/LucasLCabral/payment-service/internal/payment/models"
	"github.com/LucasLCabral/payment-service/protog/common"
	"github.com/LucasLCabral/payment-service/protog/payment"
	"github.com/google/uuid"
)

func CreatePaymentRequestFromModel(req *model.CreatePaymentRequest) *payment.CreatePaymentRequest {
	return &payment.CreatePaymentRequest{
		IdempotencyKey: req.IdempotencyKey.String(),
		AmountCents:    req.AmountCents,
		Currency:       currencyToProto(req.Currency),
		PayerId:        req.PayerID.String(),
		PayeeId:        req.PayeeID.String(),
		Description:    req.Description,
	}
}

func CreatePaymentRequestToModel(req *payment.CreatePaymentRequest) (*model.CreatePaymentRequest, error) {
	cur, ok := currencyFromProto(req.GetCurrency())
	if !ok {
		return nil, model.ErrCurrencyRequired
	}
	return model.ParseCreatePaymentRequest(
		req.GetIdempotencyKey(),
		req.GetAmountCents(),
		cur,
		req.GetPayerId(),
		req.GetPayeeId(),
		req.GetDescription(),
	)
}

func CreatePaymentResponseToPayment(resp *payment.CreatePaymentResponse) (*model.Payment, error) {
	pid, err := uuid.Parse(resp.GetPaymentId())
	if err != nil {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339, resp.GetCreatedAt())
	if err != nil {
		return nil, err
	}
	return &model.Payment{
		ID:        pid,
		Status:    paymentStatusFromProto(resp.GetStatus()),
		CreatedAt: t,
	}, nil
}

func PaymentToCreateResponse(p *model.Payment) *payment.CreatePaymentResponse {
	return &payment.CreatePaymentResponse{
		PaymentId: p.ID.String(),
		Status:    paymentStatusToProto(p.Status),
		CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func GetPaymentRequestToModel(req *payment.GetPaymentRequest) (*model.GetPaymentRequest, error) {
	pid, err := uuid.Parse(req.GetPaymentId())
	if err != nil {
		return nil, err
	}
	return &model.GetPaymentRequest{PaymentID: pid}, nil
}

func PaymentToGetResponse(p *model.Payment) *payment.GetPaymentResponse {
	return &payment.GetPaymentResponse{
		PaymentId:     p.ID.String(),
		Status:        paymentStatusToProto(p.Status),
		AmountCents:   p.AmountCents,
		Currency:      currencyToProto(p.Currency),
		CreatedAt:     p.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     p.UpdatedAt.UTC().Format(time.RFC3339),
		DeclineReason: p.DeclineReason,
	}
}

func currencyToProto(c model.Currency) common.Currency {
	switch c {
	case model.CurrencyBRL:
		return common.Currency_CURRENCY_BRL
	case model.CurrencyUSD:
		return common.Currency_CURRENCY_USD
	default:
		return common.Currency_CURRENCY_UNSPECIFIED
	}
}

func currencyFromProto(c common.Currency) (model.Currency, bool) {
	switch c {
	case common.Currency_CURRENCY_BRL:
		return model.CurrencyBRL, true
	case common.Currency_CURRENCY_USD:
		return model.CurrencyUSD, true
	default:
		return "", false
	}
}

func paymentStatusFromProto(s common.PaymentStatus) model.PaymentStatus {
	switch s {
	case common.PaymentStatus_PAYMENT_STATUS_PENDING:
		return model.PaymentStatusPending
	case common.PaymentStatus_PAYMENT_STATUS_SETTLED:
		return model.PaymentStatusSettled
	case common.PaymentStatus_PAYMENT_STATUS_DECLINED:
		return model.PaymentStatusDeclined
	case common.PaymentStatus_PAYMENT_STATUS_FAILED:
		return model.PaymentStatusFailed
	default:
		return model.PaymentStatusUnspecified
	}
}

func paymentStatusToProto(s model.PaymentStatus) common.PaymentStatus {
	switch s {
	case model.PaymentStatusPending:
		return common.PaymentStatus_PAYMENT_STATUS_PENDING
	case model.PaymentStatusSettled:
		return common.PaymentStatus_PAYMENT_STATUS_SETTLED
	case model.PaymentStatusDeclined:
		return common.PaymentStatus_PAYMENT_STATUS_DECLINED
	case model.PaymentStatusFailed:
		return common.PaymentStatus_PAYMENT_STATUS_FAILED
	default:
		return common.PaymentStatus_PAYMENT_STATUS_UNSPECIFIED
	}
}
