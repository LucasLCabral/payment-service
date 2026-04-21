package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// AttrPaymentID is the span attribute key for the payment aggregate id (UUID string).
const AttrPaymentID = "payment.id"

// AnnotatePaymentID adds payment.id to the current span, if any.
func AnnotatePaymentID(ctx context.Context, paymentID string) {
	if paymentID == "" {
		return
	}
	if span := oteltrace.SpanFromContext(ctx); span.IsRecording() {
		span.SetAttributes(attribute.String(AttrPaymentID, paymentID))
	}
}
