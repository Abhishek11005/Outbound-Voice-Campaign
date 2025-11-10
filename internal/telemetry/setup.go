package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/propagation"

	"github.com/acme/outbound-call-campaign/internal/config"
)

// Setup configures OpenTelemetry tracing and returns a shutdown function.
func Setup(ctx context.Context, cfg config.TelemetryConfig, serviceName string) (func(context.Context) error, error) {
	if !cfg.TracingEnabled {
		return func(context.Context) error { return nil }, nil
	}

	ratio := cfg.SampleRatio
	if ratio <= 0 {
		ratio = 1.0
	}

	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(cfg.Endpoint), otlptracehttp.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("otel exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	sampler := trace.ParentBased(trace.TraceIDRatioBased(ratio))
	tp := trace.NewTracerProvider(
		trace.WithSampler(sampler),
		trace.WithBatcher(exporter),
		trace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp.Shutdown, nil
}
