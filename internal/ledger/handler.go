package ledger

import (
	"context"
	"encoding/json"
	"fmt"

	pkgledger "github.com/LucasLCabral/payment-service/pkg/ledger"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/trace"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Handler struct {
	svc *Service
	log logger.Logger
}

func NewHandler(svc *Service, log logger.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

func (h *Handler) HandleMessage(ctx context.Context, msg amqp.Delivery) error {
	traceID := trace.TraceIDFromAMQPHeaders(msg.Headers)
	ctx = trace.WithTraceID(ctx, traceID.String())

	var evt pkgledger.PaymentCreatedEvent
	if err := json.Unmarshal(msg.Body, &evt); err != nil {
		h.log.Error(ctx, "invalid message payload", "err", err, "body", string(msg.Body))
		return fmt.Errorf("unmarshal: %w", err)
	}

	h.log.Info(ctx, "processing payment.created",
		"payment_id", evt.PaymentID,
		"amount_cents", evt.AmountCents,
		"currency", evt.Currency,
	)

	if err := h.svc.ProcessPaymentCreated(ctx, &evt, traceID); err != nil {
		h.log.Error(ctx, "process payment.created failed", "payment_id", evt.PaymentID, "err", err)
		return err
	}

	return nil
}

