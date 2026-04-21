package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const AttrPaymentID = "payment.id"

func AnnotatePaymentID(ctx context.Context, paymentID string) {
	if paymentID == "" {
		return
	}
	if span := oteltrace.SpanFromContext(ctx); span.IsRecording() {
		span.SetAttributes(attribute.String(AttrPaymentID, paymentID))
	}
}
