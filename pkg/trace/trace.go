package trace

import (
	"context"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type contextKey string

const (
	TraceIDKey contextKey = "trace_id"

	XTraceIDHeader = "x-trace-id"
)

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

func GetTraceID(ctx context.Context) string {
	if sc := oteltrace.SpanFromContext(ctx).SpanContext(); sc.IsValid() {
		return sc.TraceID().String()
	}
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok && traceID != "" {
		return traceID
	}
	return uuid.New().String()
}

func NewTraceID() string {
	return uuid.New().String()
}

func EnsureTraceID(ctx context.Context) context.Context {
	if sc := oteltrace.SpanFromContext(ctx).SpanContext(); sc.IsValid() {
		return WithTraceID(ctx, sc.TraceID().String())
	}
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok && traceID != "" {
		return ctx
	}
	return WithTraceID(ctx, NewTraceID())
}

func TraceIDFromAMQPHeaders(headers amqp.Table) uuid.UUID {
	if headers == nil {
		return uuid.New()
	}
	raw, ok := headers[XTraceIDHeader]
	if !ok {
		return uuid.New()
	}
	s, ok := raw.(string)
	if !ok {
		return uuid.New()
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.New()
	}
	return id
}

func GetTraceUUID(ctx context.Context) uuid.UUID {
	if sc := oteltrace.SpanFromContext(ctx).SpanContext(); sc.IsValid() {
		return uuid.UUID(sc.TraceID())
	}
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok && traceID != "" {
		if id, err := uuid.Parse(traceID); err == nil {
			return id
		}
	}
	return uuid.New()
}
