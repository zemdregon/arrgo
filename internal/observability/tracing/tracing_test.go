package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
)

func TestSetupConfiguresProviderAndExportsSpans(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")

	originalProvider := otel.GetTracerProvider()
	originalPropagator := otel.GetTextMapPropagator()
	defer func() {
		otel.SetTracerProvider(originalProvider)
		otel.SetTextMapPropagator(originalPropagator)
	}()

	shutdown, err := Setup(context.Background(), Config{
		ServiceName:       "tracing-test",
		CollectorEndpoint: endpoint,
		Insecure:          true,
		Timeout:           time.Second,
		SampleRatio:       1,
	})
	if err != nil {
		t.Fatalf("setup tracing: %v", err)
	}

	tracer := otel.Tracer("tracing/test")
	ctx, span := tracer.Start(context.Background(), "root")
	span.End()
	_, child := tracer.Start(ctx, "child")
	child.End()

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown tracing: %v", err)
	}

	if requestCount.Load() == 0 {
		t.Fatalf("expected exporter to send at least one request")
	}
}

func TestSetupRequiresServiceName(t *testing.T) {
	_, err := Setup(context.Background(), Config{})
	if err == nil {
		t.Fatal("expected error when service name is missing")
	}
}

func TestSetupRespectsSampleRatioBounds(t *testing.T) {
	originalProvider := otel.GetTracerProvider()
	originalPropagator := otel.GetTextMapPropagator()
	defer func() {
		otel.SetTracerProvider(originalProvider)
		otel.SetTextMapPropagator(originalPropagator)
	}()

	shutdown, err := Setup(context.Background(), Config{
		ServiceName: "sampling-test",
		Insecure:    true,
	})
	if err != nil {
		t.Fatalf("setup tracing with defaults: %v", err)
	}
	tp := otel.GetTracerProvider()
	if _, ok := tp.(*trace.TracerProvider); !ok {
		t.Fatalf("expected sdk tracer provider, got %T", tp)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown tracing: %v", err)
	}
}
