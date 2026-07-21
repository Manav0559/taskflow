// Package tracing wires a global OpenTelemetry TracerProvider so all three services
// (api, worker, scheduler) export spans the same way. Each service's HTTP/DB/handler
// work gets its own span, letting a real collector (e.g. Jaeger) show, for a single
// request, how long was spent in the API handler vs. each Postgres query.
//
// Known limitation (see docs/ARCHITECTURE.md): spans are per-service, not yet linked
// into one end-to-end trace per job. Doing that would mean persisting the originating
// trace ID on the job at creation time and continuing it when the scheduler promotes
// the run and when the worker executes it — a real next step, not implemented here.
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Init configures tracing for serviceName. If otlpEndpoint is empty, tracing is a
// no-op (the global no-op TracerProvider stays in place) so every service runs fine
// with no collector present — this is deliberately opt-in, not a hard dependency.
func Init(ctx context.Context, serviceName, otlpEndpoint string) (shutdown func(context.Context) error, err error) {
	if otlpEndpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(otlpEndpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	res := resource.NewSchemaless(attribute.String("service.name", serviceName))

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}
