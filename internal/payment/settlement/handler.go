package settlement

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/LucasLCabral/payment-service/internal/payment/repository"
	pkgledger "github.com/LucasLCabral/payment-service/pkg/ledger"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/messaging"
	"github.com/LucasLCabral/payment-service/pkg/payment"
	"github.com/LucasLCabral/payment-service/pkg/telemetry"
	"github.com/LucasLCabral/payment-service/pkg/trace"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

//go:generate go run go.uber.org/mock/mockgen -destination=mocks/transaction_runner_mock.go -package=mocks . TransactionRunner
//go:generate go run go.uber.org/mock/mockgen -destination=mocks/payment_status_notifier_mock.go -package=mocks . PaymentStatusNotifier
//go:generate go run go.uber.org/mock/mockgen -destination=mocks/payment_repository_mock.go -package=mocks github.com/LucasLCabral/payment-service/internal/payment/repository Payment
//go:generate go run go.uber.org/mock/mockgen -destination=mocks/audit_repository_mock.go -package=mocks github.com/LucasLCabral/payment-service/internal/payment/repository Audit

type TransactionRunner interface {
	WithinTransaction(ctx context.Context, fn func(tx *sql.Tx) error) error
}

type PaymentStatusNotifier interface {
	NotifyPaymentStatus(ctx context.Context, paymentID uuid.UUID, status payment.PaymentStatus, declineReason string) error
}

type Handler struct {
	tx       TransactionRunner
	repo     repository.Payment
	audit    repository.Audit
	log      logger.Logger
	notifier PaymentStatusNotifier
}

func NewHandler(tx TransactionRunner, repo repository.Payment, audit repository.Audit, log logger.Logger, notifier PaymentStatusNotifier) *Handler {
	return &Handler{tx: tx, repo: repo, audit: audit, log: log, notifier: notifier}
}

func (h *Handler) HandleMessage(ctx context.Context, msg amqp.Delivery) error {
	ctx = telemetry.ExtractAMQPContext(ctx, msg.Headers)

	tracer := otel.Tracer("payment-service")
	ctx, span := tracer.Start(ctx, "settlement consume",
		oteltrace.WithSpanKind(oteltrace.SpanKindConsumer),
	)
	defer span.End()

	traceID := trace.TraceIDFromAMQPHeaders(msg.Headers)
	ctx = trace.WithTraceID(ctx, traceID.String())

	var evt pkgledger.SettlementResult
	if err := json.Unmarshal(msg.Body, &evt); err != nil {
		h.log.Error(ctx, "invalid settlement payload", "err", err, "body", string(msg.Body))
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return messaging.Permanent(fmt.Errorf("unmarshal: %w", err))
	}
	telemetry.AnnotatePaymentID(ctx, evt.PaymentID.String())

	newStatus := mapStatus(evt.Status)
	if newStatus == payment.StatusUnspecified {
		err := fmt.Errorf("unknown settlement status: %s", evt.Status)
		h.log.Error(ctx, "unknown settlement status", "status", evt.Status, "payment_id", evt.PaymentID)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return messaging.Permanent(err)
	}

	h.log.Info(ctx, "processing settlement",
		"payment_id", evt.PaymentID,
		"status", evt.Status,
	)

	err := h.tx.WithinTransaction(ctx, func(tx *sql.Tx) error {
		if err := h.repo.UpdateStatus(ctx, tx, evt.PaymentID, newStatus, evt.DeclineReason); err != nil {
			return fmt.Errorf("update status: %w", err)
		}

		return h.audit.Insert(ctx, tx, &payment.AuditEntry{
			EntityType: "payment",
			EntityID:   evt.PaymentID,
			Action:     "settlement." + evt.Status,
			OldStatus:  payment.StatusPending,
			NewStatus:  newStatus,
			TraceID:    traceID,
			Actor:      "ledger-service",
		})
	})
	if err != nil {
		h.log.Error(ctx, "settlement processing failed", "payment_id", evt.PaymentID, "err", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	h.log.Info(ctx, "settlement applied", "payment_id", evt.PaymentID, "new_status", newStatus)
	if h.notifier != nil {
		if err := h.notifier.NotifyPaymentStatus(ctx, evt.PaymentID, newStatus, evt.DeclineReason); err != nil {
			h.log.Warn(ctx, "payment status notify failed", "payment_id", evt.PaymentID, "err", err)
		}
	}
	span.SetStatus(codes.Ok, "")
	return nil
}

func mapStatus(s string) payment.PaymentStatus {
	switch s {
	case "SETTLED":
		return payment.StatusSettled
	case "DECLINED":
		return payment.StatusDeclined
	default:
		return payment.StatusUnspecified
	}
}
