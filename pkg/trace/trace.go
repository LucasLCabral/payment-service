package trace

import (
	"context"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
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
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok && traceID != "" {
		return traceID
	}
	return uuid.New().String()
}

func NewTraceID() string {
	return uuid.New().String()
}

func EnsureTraceID(ctx context.Context) context.Context {
	if traceID := GetTraceID(ctx); traceID != "" {
		return WithTraceID(ctx, traceID)
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
	s := GetTraceID(ctx)
	id, err := uuid.Parse(s)
	if err == nil {
		return id
	}
	return uuid.New()
}
