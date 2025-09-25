package logging

import (
	"context"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

type contextKey string

const (
	jobIDKey contextKey = "arrgo.job_id"
)

// WithContext returns a logger enriched with telemetry identifiers from the context.
// When the context contains an active OpenTelemetry span the logger is decorated with
// the corresponding trace identifier. If a job identifier has been propagated via
// ContextWithJobID, the logger will also include that attribute so downstream logs can
// be correlated across services.
func WithContext(ctx context.Context, logger *slog.Logger) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	if ctx == nil {
		return logger
	}

	var attrs []any
	if spanCtx := trace.SpanContextFromContext(ctx); spanCtx.IsValid() {
		attrs = append(attrs, slog.String("trace_id", spanCtx.TraceID().String()))
		if spanCtx.HasSpanID() {
			attrs = append(attrs, slog.String("span_id", spanCtx.SpanID().String()))
		}
	}

	if jobID, ok := JobIDFromContext(ctx); ok {
		attrs = append(attrs, slog.String("job_id", jobID))
	}

	if len(attrs) == 0 {
		return logger
	}
	return logger.With(attrs...)
}

// ContextWithJobID attaches a job identifier to the context so downstream loggers can
// automatically annotate structured logs.
func ContextWithJobID(ctx context.Context, jobID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	trimmed := strings.TrimSpace(jobID)
	if trimmed == "" {
		return ctx
	}
	return context.WithValue(ctx, jobIDKey, trimmed)
}

// JobIDFromContext retrieves a job identifier from the context if one has been set.
func JobIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	raw := ctx.Value(jobIDKey)
	jobID, ok := raw.(string)
	if !ok {
		return "", false
	}
	trimmed := strings.TrimSpace(jobID)
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}
