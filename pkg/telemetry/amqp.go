package telemetry

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// InjectAMQPHeaders copies W3C trace context from ctx into h (traceparent, tracestate).
func InjectAMQPHeaders(ctx context.Context, h map[string]interface{}) {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	for k, v := range carrier {
		h[k] = v
	}
}

// ExtractAMQPContext returns parent with the remote span from AMQP headers (if any).
func ExtractAMQPContext(parent context.Context, headers amqp.Table) context.Context {
	c := propagation.MapCarrier{}
	for k, v := range headers {
		if s, ok := v.(string); ok {
			c[k] = s
		}
	}
	return otel.GetTextMapPropagator().Extract(parent, c)
}
