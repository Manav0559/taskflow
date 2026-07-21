package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// otelQueryTracer implements pgx.QueryTracer so every query the pool runs (across
// every store method) produces a span automatically — instrumenting call sites one by
// one would mean touching all ~25 Store methods for the same result.
type otelQueryTracer struct {
	tracer trace.Tracer
}

type traceCtxKey struct{}

func newOtelQueryTracer() *otelQueryTracer {
	return &otelQueryTracer{tracer: otel.Tracer("taskflow/store")}
}

func (t *otelQueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	spanCtx, span := t.tracer.Start(ctx, "pg.query", trace.WithAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.statement", data.SQL),
	))
	return context.WithValue(spanCtx, traceCtxKey{}, span)
}

func (t *otelQueryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span, ok := ctx.Value(traceCtxKey{}).(trace.Span)
	if !ok {
		return
	}
	if data.Err != nil {
		span.RecordError(data.Err)
		span.SetStatus(codes.Error, data.Err.Error())
	}
	span.End()
}
