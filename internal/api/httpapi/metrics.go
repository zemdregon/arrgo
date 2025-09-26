package httpapi

import (
	"context"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type httpMetrics struct {
	requestCounter    metric.Int64Counter
	durationHistogram metric.Float64Histogram
}

var (
	metricsOnce       sync.Once
	globalHTTPMetrics *httpMetrics
)

func newHTTPMetrics() *httpMetrics {
	metricsOnce.Do(func() {
		meter := otel.Meter("github.com/arrgo/arrgo/internal/api/httpapi")

		counter, err := meter.Int64Counter("arrgo_http_requests_total",
			metric.WithDescription("Total number of HTTP requests handled by the arrgo master API."),
			metric.WithUnit("{requests}"),
		)
		if err != nil {
			otel.Handle(err)
			return
		}

		histogram, err := meter.Float64Histogram("arrgo_http_request_duration_seconds",
			metric.WithDescription("HTTP request duration observed by the arrgo master API."),
			metric.WithUnit("s"),
		)
		if err != nil {
			otel.Handle(err)
			return
		}

		globalHTTPMetrics = &httpMetrics{
			requestCounter:    counter,
			durationHistogram: histogram,
		}
	})

	return globalHTTPMetrics
}

func (m *httpMetrics) record(ctx context.Context, route, method string, status int, duration time.Duration) {
	if m == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("route", route),
		attribute.String("method", method),
		attribute.String("status", strconv.Itoa(status)),
	}
	m.requestCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.durationHistogram.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}
