package metadata

import "github.com/prometheus/client_golang/prometheus"

// Metrics encapsulates Prometheus collectors for the metadata ingestion pipeline.
type Metrics struct {
	providerFailures *prometheus.CounterVec
	rateLimitRetries *prometheus.CounterVec
}

// NewMetrics registers metadata ingestion metrics with the supplied registry.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		providerFailures: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "arrgo",
				Subsystem: "metadata",
				Name:      "provider_failures_total",
				Help:      "Number of provider fetch failures grouped by provider and reason.",
			},
			[]string{"provider", "reason"},
		),
		rateLimitRetries: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "arrgo",
				Subsystem: "metadata",
				Name:      "rate_limit_retries_total",
				Help:      "Number of provider retries triggered by rate limiting.",
			},
			[]string{"provider"},
		),
	}

	if reg != nil {
		reg.MustRegister(m.providerFailures, m.rateLimitRetries)
	}

	return m
}

// ProviderFailures exposes the counter recording provider failures.
func (m *Metrics) ProviderFailures() *prometheus.CounterVec { return m.providerFailures }

// RateLimitRetries exposes the counter tracking rate limit induced retries.
func (m *Metrics) RateLimitRetries() *prometheus.CounterVec { return m.rateLimitRetries }
