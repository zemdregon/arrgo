package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestWithContextAddsTraceAndSpanIdentifiers(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, nil))

	traceID := trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8}
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanCtx)

	WithContext(ctx, logger).Info("hello", slog.String("key", "value"))

	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("decode log payload: %v", err)
	}

	if got := payload["trace_id"]; got != traceID.String() {
		t.Fatalf("expected trace_id %s, got %v", traceID.String(), got)
	}
	if got := payload["span_id"]; got != spanID.String() {
		t.Fatalf("expected span_id %s, got %v", spanID.String(), got)
	}
}

func TestContextWithJobIDEnrichesLogger(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, nil))

	ctx := ContextWithJobID(context.Background(), " job-123 ")
	ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1},
		SpanID:  trace.SpanID{1},
	}))

	WithContext(ctx, logger).Info("test")

	var payload map[string]any
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("decode log payload: %v", err)
	}

	if got := payload["job_id"]; got != "job-123" {
		t.Fatalf("expected job_id job-123, got %v", got)
	}
}

func TestJobIDFromContextAbsent(t *testing.T) {
	if _, ok := JobIDFromContext(context.Background()); ok {
		t.Fatalf("expected no job id in empty context")
	}
}

func TestContextWithJobIDIgnoresWhitespaceOnly(t *testing.T) {
	ctx := ContextWithJobID(context.Background(), "   ")
	if _, ok := JobIDFromContext(ctx); ok {
		t.Fatalf("expected whitespace job id to be ignored")
	}
}
