package ledger

import (
	"context"
	"encoding/json"
	"fmt"

	pkgledger "github.com/LucasLCabral/payment-service/pkg/ledger"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/telemetry"
	"github.com/LucasLCabral/payment-service/pkg/trace"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type Handler struct {
	svc *Service
	log logger.Logger
}

func NewHandler(svc *Service, log logger.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

func (h *Handler) HandleMessage(ctx context.Context, msg amqp.Delivery) error {
	ctx = telemetry.ExtractAMQPContext(ctx, msg.Headers)

	tracer := otel.Tracer("ledger-service")
	ctx, span := tracer.Start(ctx, "payment.created consume",
		oteltrace.WithSpanKind(oteltrace.SpanKindConsumer),
	)
	defer span.End()

	traceID := trace.TraceIDFromAMQPHeaders(msg.Headers)
	ctx = trace.WithTraceID(ctx, traceID.String())

	var evt pkgledger.PaymentCreatedEvent
	if err := json.Unmarshal(msg.Body, &evt); err != nil {
		h.log.Error(ctx, "invalid message payload", "err", err, "body", string(msg.Body))
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("unmarshal: %w", err)
	}
	telemetry.AnnotatePaymentID(ctx, evt.PaymentID.String())

	h.log.Info(ctx, "processing payment.created",
		"payment_id", evt.PaymentID,
		"amount_cents", evt.AmountCents,
		"currency", evt.Currency,
	)

	if err := h.svc.ProcessPaymentCreated(ctx, &evt, traceID); err != nil {
		h.log.Error(ctx, "process payment.created failed", "payment_id", evt.PaymentID, "err", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetStatus(codes.Ok, "")
	return nil
}

