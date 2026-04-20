package trace

import (
	"context"

	"github.com/google/uuid"
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
