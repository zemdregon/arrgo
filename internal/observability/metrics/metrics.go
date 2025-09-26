package metrics

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const (
	defaultExportInterval = 15 * time.Second
	shutdownTimeout       = 5 * time.Second
)

// Config describes metric export configuration for arrgo services.
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
	// ExportInterval defines how frequently metrics are pushed to the collector.
	ExportInterval time.Duration
}

// Setup configures a global OpenTelemetry meter provider that exports metrics to the
// configured collector. It returns a shutdown function that flushes metrics on exit.
func Setup(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if strings.TrimSpace(cfg.ServiceName) == "" {
		return nil, errors.New("metrics: service name is required")
	}

	exporterOpts := []otlpmetrichttp.Option{}
	if cfg.CollectorEndpoint != "" {
		exporterOpts = append(exporterOpts, otlpmetrichttp.WithEndpoint(cfg.CollectorEndpoint))
	}
	if cfg.Insecure {
		exporterOpts = append(exporterOpts, otlpmetrichttp.WithInsecure())
	}
	if cfg.Timeout > 0 {
		exporterOpts = append(exporterOpts, otlpmetrichttp.WithTimeout(cfg.Timeout))
	}

	exporter, err := otlpmetrichttp.New(ctx, exporterOpts...)
	if err != nil {
		return nil, fmt.Errorf("metrics: create otlp exporter: %w", err)
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
		return nil, fmt.Errorf("metrics: build resource: %w", err)
	}

	interval := defaultExportInterval
	if cfg.ExportInterval > 0 {
		interval = cfg.ExportInterval
	}

	reader := sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(interval))

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	)

	otel.SetMeterProvider(provider)

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
		if err := provider.ForceFlush(ctx); err != nil {
			return fmt.Errorf("metrics: force flush: %w", err)
		}
		if err := provider.Shutdown(ctx); err != nil {
			return fmt.Errorf("metrics: shutdown provider: %w", err)
		}
		return nil
	}

	return shutdown, nil
}

// CollectOnce exposes a helper for tests to collect metrics from a manual reader.
// It is placed here to avoid duplicating boilerplate in multiple packages.
func CollectOnce(ctx context.Context, reader *sdkmetric.ManualReader) (metricdata.ResourceMetrics, error) {
	var out metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &out); err != nil {
		return metricdata.ResourceMetrics{}, err
	}
	return out, nil
}
