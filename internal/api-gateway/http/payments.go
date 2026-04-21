package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/payment"
	"github.com/google/uuid"
)

type PaymentService interface {
	CreatePayment(ctx context.Context, in *payment.CreatePaymentRequest) (*payment.Payment, error)
	GetPayment(ctx context.Context, req *payment.GetPaymentRequest) (*payment.Payment, error)
}

type PaymentsHandler struct {
	log     logger.Logger
	payment PaymentService
}

func NewPaymentsHandler(log logger.Logger, p PaymentService) *PaymentsHandler {
	return &PaymentsHandler{log: log, payment: p}
}

func (h *PaymentsHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.payment == nil {
		writeError(w, ctx, h.log, http.StatusServiceUnavailable, "payment service unavailable")
		return
	}

	var body struct {
		IdempotencyKey string `json:"idempotency_key"`
		AmountCents    int64  `json:"amount_cents"`
		Currency       string `json:"currency"`
		PayerID        string `json:"payer_id"`
		PayeeID        string `json:"payee_id"`
		Description    string `json:"description,omitempty"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, ctx, h.log, http.StatusBadRequest, "invalid json")
		return
	}

	cur, ok := parseCurrency(body.Currency)
	if !ok {
		writeError(w, ctx, h.log, http.StatusBadRequest, "currency must be BRL or USD")
		return
	}

	in, err := payment.ParseCreatePaymentRequest(
		body.IdempotencyKey,
		body.AmountCents,
		cur,
		body.PayerID,
		body.PayeeID,
		body.Description,
	)
	if err != nil {
		writeError(w, ctx, h.log, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	res, err := h.payment.CreatePayment(ctx, in)
	if err != nil {
		handleGRPCError(w, ctx, h.log, err)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"payment_id": res.ID.String(),
		"status":     string(res.Status),
		"created_at": res.CreatedAt.UTC().Format(time.RFC3339),
	})
}

func (h *PaymentsHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.payment == nil {
		writeError(w, ctx, h.log, http.StatusServiceUnavailable, "payment service unavailable")
		return
	}

	id := r.PathValue("id")
	paymentID, err := uuid.Parse(id)
	if err != nil {
		writeError(w, ctx, h.log, http.StatusBadRequest, "invalid payment id")
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	res, err := h.payment.GetPayment(ctx, &payment.GetPaymentRequest{PaymentID: paymentID})
	if err != nil {
		handleGRPCError(w, ctx, h.log, err)
		return
	}

	resp := map[string]any{
		"payment_id":  res.ID.String(),
		"status":      string(res.Status),
		"amount_cents": res.AmountCents,
		"currency":    string(res.Currency),
		"payer_id":    res.PayerID.String(),
		"payee_id":    res.PayeeID.String(),
		"created_at":  res.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":  res.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if res.Description != "" {
		resp["description"] = res.Description
	}
	if res.DeclineReason != "" {
		resp["decline_reason"] = res.DeclineReason
	}

	writeJSON(w, http.StatusOK, resp)
}
