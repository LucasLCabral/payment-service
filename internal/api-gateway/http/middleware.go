package http

import (
	nethttp "net/http"
	"time"

	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/LucasLCabral/payment-service/pkg/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type statusWriter struct {
	nethttp.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func LoggingMiddleware(log logger.Logger) func(nethttp.Handler) nethttp.Handler {
	return func(next nethttp.Handler) nethttp.Handler {
		return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
			start := time.Now()

			ctx := r.Context()
			traceID := ""
			sc := oteltrace.SpanFromContext(ctx).SpanContext()
			if sc.IsValid() {
				traceID = sc.TraceID().String()
			} else if t := r.Header.Get(trace.XTraceIDHeader); t != "" {
				traceID = t
			}
			ctx = trace.WithTraceID(ctx, traceID)
			r = r.WithContext(ctx)

			sw := &statusWriter{ResponseWriter: w, status: nethttp.StatusOK}
			next.ServeHTTP(sw, r)

			log.Info(ctx, "http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}
