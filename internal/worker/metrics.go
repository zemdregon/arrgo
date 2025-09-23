package worker

import "github.com/prometheus/client_golang/prometheus"

// Metrics encapsulates the Prometheus collectors owned by the worker service.
type Metrics struct {
	activeWorkers *prometheus.GaugeVec
	heartbeatAge  *prometheus.GaugeVec
	staleWorkers  *prometheus.GaugeVec
}

// NewMetrics registers worker-related metrics with the supplied registry.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		activeWorkers: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "arrgo",
				Subsystem: "workers",
				Name:      "active",
				Help:      "Number of healthy workers per pool.",
			},
			[]string{"pool"},
		),
		heartbeatAge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "arrgo",
				Subsystem: "workers",
				Name:      "heartbeat_seconds",
				Help:      "Age in seconds of the most recent worker heartbeat.",
			},
			[]string{"pool", "worker_id"},
		),
		staleWorkers: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "arrgo",
				Subsystem: "workers",
				Name:      "stale",
				Help:      "Number of workers whose heartbeat exceeded the staleness threshold.",
			},
			[]string{"pool"},
		),
	}

	if reg != nil {
		reg.MustRegister(m.activeWorkers, m.heartbeatAge, m.staleWorkers)
	}

	return m
}

// ActiveWorkers returns the gauge reporting healthy workers per pool.
func (m *Metrics) ActiveWorkers() *prometheus.GaugeVec { return m.activeWorkers }

// HeartbeatAge exposes the gauge tracking heartbeat ages per worker.
func (m *Metrics) HeartbeatAge() *prometheus.GaugeVec { return m.heartbeatAge }

// StaleWorkers returns the gauge tracking stale workers per pool.
func (m *Metrics) StaleWorkers() *prometheus.GaugeVec { return m.staleWorkers }
