package tracing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const (
	defaultSampleRatio = 0.10
	shutdownTimeout    = 5 * time.Second
)

// Config describes tracing configuration for arrgo services.
type Config struct {
	// ServiceName identifies the service emitting telemetry (e.g., "arrgo-master").
	ServiceName string
	// ServiceVersion captures the running build's version string.
	ServiceVersion string
	// Environment differentiates deployments (e.g., "development", "production").
	Environment string
	// CollectorEndpoint is the OTLP/HTTP endpoint (host:port) for the collector.
	CollectorEndpoint string
	// Insecure disables TLS when communicating with the collector (useful for local dev).
	Insecure bool
	// Timeout bounds exporter startup/shutdown interactions with the collector.
	Timeout time.Duration
	// SampleRatio overrides the default parent-based 10% sampling probability.
	// Values must be in the range (0, 1].
	SampleRatio float64
}

// Setup configures a global OpenTelemetry tracer provider and W3C propagators.
// It returns a shutdown function that flushes spans to the configured collector.
func Setup(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if strings.TrimSpace(cfg.ServiceName) == "" {
		return nil, errors.New("tracing: service name is required")
	}

	clientOpts := []otlptracehttp.Option{}
	if cfg.CollectorEndpoint != "" {
		clientOpts = append(clientOpts, otlptracehttp.WithEndpoint(cfg.CollectorEndpoint))
	}
	if cfg.Insecure {
		clientOpts = append(clientOpts, otlptracehttp.WithInsecure())
	}
	if cfg.Timeout > 0 {
		clientOpts = append(clientOpts, otlptracehttp.WithTimeout(cfg.Timeout))
	}

	exporter, err := otlptracehttp.New(ctx, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("tracing: create otlp exporter: %w", err)
	}

	attributes := []attribute.KeyValue{
		semconv.ServiceName(cfg.ServiceName),
	}
	if cfg.ServiceVersion != "" {
		attributes = append(attributes, semconv.ServiceVersion(cfg.ServiceVersion))
	}
	if cfg.Environment != "" {
		attributes = append(attributes, attribute.String("deployment.environment", cfg.Environment))
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(attributes...),
	)
	if err != nil {
		_ = exporter.Shutdown(context.Background())
		return nil, fmt.Errorf("tracing: build resource: %w", err)
	}

	ratio := defaultSampleRatio
	if cfg.SampleRatio > 0 && cfg.SampleRatio <= 1 {
		ratio = cfg.SampleRatio
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	shutdown := func(ctx context.Context) error {
		if ctx == nil {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()
		} else if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, shutdownTimeout)
			defer cancel()
		}
		return tp.Shutdown(ctx)
	}

	return shutdown, nil
}
