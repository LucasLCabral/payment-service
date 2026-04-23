package ledger

import (
	"context"
	"encoding/json"
	"fmt"

	pkgledger "github.com/LucasLCabral/payment-service/pkg/ledger"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/messaging"
	"github.com/LucasLCabral/payment-service/pkg/telemetry"
	"github.com/LucasLCabral/payment-service/pkg/trace"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

//go:generate go run go.uber.org/mock/mockgen -destination=mocks/payment_created_processor_mock.go -package=mocks . PaymentCreatedProcessor

type PaymentCreatedProcessor interface {
	ProcessPaymentCreated(ctx context.Context, evt *pkgledger.PaymentCreatedEvent, traceID uuid.UUID) error
}

type Handler struct {
	proc PaymentCreatedProcessor
	log  logger.Logger
}

func NewHandler(proc PaymentCreatedProcessor, log logger.Logger) *Handler {
	return &Handler{proc: proc, log: log}
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
		return messaging.Permanent(fmt.Errorf("unmarshal: %w", err))
	}
	telemetry.AnnotatePaymentID(ctx, evt.PaymentID.String())

	h.log.Info(ctx, "processing payment.created",
		"payment_id", evt.PaymentID,
		"amount_cents", evt.AmountCents,
		"currency", evt.Currency,
	)

	if err := h.proc.ProcessPaymentCreated(ctx, &evt, traceID); err != nil {
		h.log.Error(ctx, "process payment.created failed", "payment_id", evt.PaymentID, "err", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetStatus(codes.Ok, "")
	return nil
}

