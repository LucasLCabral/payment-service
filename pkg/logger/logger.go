package logger

import (
	"context"
	"log/slog"
	"os"

	"github.com/LucasLCabral/payment-service/pkg/trace"
)

type Logger interface {
	Debug(ctx context.Context, msg string, args ...any)
	Info(ctx context.Context, msg string, args ...any)
	Warn(ctx context.Context, msg string, args ...any)
	Error(ctx context.Context, msg string, args ...any)
	With(args ...any) Logger
}

type slogLogger struct {
	logger *slog.Logger
}

func New(serviceName string) Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	logger := slog.New(handler).With(
		slog.String("service", serviceName),
	)

	return &slogLogger{logger: logger}
}

func (l *slogLogger) withTraceID(ctx context.Context, args []any) []any {
	return append([]any{slog.String("trace_id", trace.TraceIDForLog(ctx))}, args...)
}

func (l *slogLogger) Debug(ctx context.Context, msg string, args ...any) {
	l.logger.DebugContext(ctx, msg, l.withTraceID(ctx, args)...)
}

func (l *slogLogger) Info(ctx context.Context, msg string, args ...any) {
	l.logger.InfoContext(ctx, msg, l.withTraceID(ctx, args)...)
}

func (l *slogLogger) Warn(ctx context.Context, msg string, args ...any) {
	l.logger.WarnContext(ctx, msg, l.withTraceID(ctx, args)...)
}

func (l *slogLogger) Error(ctx context.Context, msg string, args ...any) {
	l.logger.ErrorContext(ctx, msg, l.withTraceID(ctx, args)...)
}

func (l *slogLogger) With(args ...any) Logger {
	return &slogLogger{
		logger: l.logger.With(args...),
	}
}
