package indexer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"
)

// Outcome captures the result of an indexer request.
type Outcome string

const (
	// OutcomeSuccess represents a successful indexer response.
	OutcomeSuccess Outcome = "success"
	// OutcomeFailure represents a failed indexer response (e.g., HTTP 5xx).
	OutcomeFailure Outcome = "failure"
	// OutcomeTimeout represents a request that timed out or otherwise never completed.
	OutcomeTimeout Outcome = "timeout"
)

// Clock abstracts time retrieval for deterministic testing.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// Observation captures a single interaction with an indexer.
type Observation struct {
	Indexer   string
	Outcome   Outcome
	Latency   time.Duration
	Error     error
	Timestamp time.Time
}

// Metadata describes a discovered indexer.
type Metadata struct {
	Name       string
	Endpoint   string
	Categories []string
	Tags       map[string]string
}

// Discoverer locates indexers dynamically.
type Discoverer interface {
	DiscoverIndexers(context.Context) ([]Metadata, error)
}

// Config controls Monitor behaviour.
type Config struct {
	SampleWindow                     int
	FailurePenalty                   float64
	TimeoutPenalty                   float64
	LatencyPenalty                   float64
	LatencyTarget                    time.Duration
	HealthyScore                     float64
	DegradedScore                    float64
	FailoverFloorScore               float64
	MinObservationsForRecommendation int
	Clock                            Clock
	Logger                           *slog.Logger
	Discoverers                      []Discoverer
}

// Errors exposed by the monitor.
var (
	ErrObservationIndexerRequired = errors.New("indexer: observation indexer name required")
	ErrObservationOutcomeInvalid  = errors.New("indexer: invalid observation outcome")
)

const (
	defaultSampleWindow    = 50
	defaultFailurePenalty  = 40.0
	defaultTimeoutPenalty  = 70.0
	defaultLatencyPenalty  = 15.0
	defaultLatencyTarget   = 500 * time.Millisecond
	defaultHealthyScore    = 80.0
	defaultDegradedScore   = 60.0
	defaultMinObservations = 5
)

// Monitor tracks indexer health and discovery state.
type Monitor struct {
	cfg    Config
	logger *slog.Logger
	clock  Clock

	mu          sync.RWMutex
	indexers    map[string]*indexerState
	discoverers []Discoverer
}

// NewMonitor constructs a Monitor using the supplied configuration.
func NewMonitor(cfg Config) *Monitor {
	if cfg.SampleWindow <= 0 {
		cfg.SampleWindow = defaultSampleWindow
	}
	if cfg.FailurePenalty <= 0 {
		cfg.FailurePenalty = defaultFailurePenalty
	}
	if cfg.TimeoutPenalty <= 0 {
		cfg.TimeoutPenalty = defaultTimeoutPenalty
	}
	if cfg.LatencyPenalty <= 0 {
		cfg.LatencyPenalty = defaultLatencyPenalty
	}
	if cfg.LatencyTarget <= 0 {
		cfg.LatencyTarget = defaultLatencyTarget
	}
	if cfg.HealthyScore <= 0 {
		cfg.HealthyScore = defaultHealthyScore
	}
	if cfg.DegradedScore <= 0 {
		cfg.DegradedScore = defaultDegradedScore
	}
	if cfg.FailoverFloorScore <= 0 {
		cfg.FailoverFloorScore = cfg.DegradedScore
	}
	if cfg.MinObservationsForRecommendation <= 0 {
		cfg.MinObservationsForRecommendation = defaultMinObservations
	}

	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	monitor := &Monitor{
		cfg:         cfg,
		logger:      logger,
		clock:       clock,
		indexers:    make(map[string]*indexerState),
		discoverers: make([]Discoverer, 0, len(cfg.Discoverers)),
	}
	monitor.discoverers = append(monitor.discoverers, cfg.Discoverers...)
	return monitor
}

// RegisterDiscoverer appends a discovery source to the monitor.
func (m *Monitor) RegisterDiscoverer(d Discoverer) {
	if d == nil {
		return
	}
	m.mu.Lock()
	m.discoverers = append(m.discoverers, d)
	m.mu.Unlock()
}

// Record ingests an indexer observation, returning true when the indexer was newly discovered.
func (m *Monitor) Record(obs Observation) (bool, error) {
	if obs.Indexer == "" {
		return false, ErrObservationIndexerRequired
	}
	if obs.Outcome != OutcomeSuccess && obs.Outcome != OutcomeFailure && obs.Outcome != OutcomeTimeout {
		return false, ErrObservationOutcomeInvalid
	}
	if obs.Timestamp.IsZero() {
		obs.Timestamp = m.clock.Now()
	}
	if obs.Latency < 0 {
		obs.Latency = 0
	}

	state, created := m.ensureState(obs.Indexer, obs.Timestamp)
	if created {
		m.logger.Info("discovered indexer via observation", "indexer", obs.Indexer)
	}
	state.record(obs, m.cfg)
	return created, nil
}

// Discover queries registered discoverers and updates metadata, returning the unique discovered set.
func (m *Monitor) Discover(ctx context.Context) ([]Metadata, error) {
	m.mu.RLock()
	discoverers := append([]Discoverer(nil), m.discoverers...)
	m.mu.RUnlock()

	if len(discoverers) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{})
	results := make([]Metadata, 0)
	now := m.clock.Now()

	for _, d := range discoverers {
		metas, err := d.DiscoverIndexers(ctx)
		if err != nil {
			return nil, err
		}
		for _, meta := range metas {
			if meta.Name == "" {
				continue
			}
			state, created := m.ensureState(meta.Name, now)
			if created {
				m.logger.Info("discovered indexer via discovery", "indexer", meta.Name)
			}
			state.updateMetadata(meta)
			if _, ok := seen[meta.Name]; ok {
				continue
			}
			seen[meta.Name] = struct{}{}
			results = append(results, meta)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Name == results[j].Name {
			return results[i].Endpoint < results[j].Endpoint
		}
		return results[i].Name < results[j].Name
	})
	return results, nil
}

// HealthReport summarizes health statistics for a single indexer.
type HealthReport struct {
	Name           string
	Score          float64
	Healthy        bool
	Availability   float64
	FailureRate    float64
	TimeoutRate    float64
	AverageLatency time.Duration
	Observations   int
	LastObserved   time.Time
	LastError      string
	DiscoveredAt   time.Time
	Metadata       Metadata
}

// Snapshot returns the current health reports for all tracked indexers.
func (m *Monitor) Snapshot() []HealthReport {
	m.mu.RLock()
	states := make([]*indexerState, 0, len(m.indexers))
	for _, state := range m.indexers {
		states = append(states, state)
	}
	m.mu.RUnlock()

	reports := make([]HealthReport, 0, len(states))
	for _, state := range states {
		snap := state.snapshot(m.cfg)
		reports = append(reports, HealthReport{
			Name:           state.name,
			Score:          snap.score,
			Healthy:        snap.score >= m.cfg.HealthyScore,
			Availability:   snap.successRate,
			FailureRate:    snap.failureRate,
			TimeoutRate:    snap.timeoutRate,
			AverageLatency: snap.averageLatency,
			Observations:   snap.count,
			LastObserved:   snap.lastObserved,
			LastError:      snap.lastError,
			DiscoveredAt:   snap.discoveredAt,
			Metadata:       snap.metadata,
		})
	}

	sort.Slice(reports, func(i, j int) bool {
		if reports[i].Score == reports[j].Score {
			return reports[i].Name < reports[j].Name
		}
		return reports[i].Score > reports[j].Score
	})
	return reports
}

// FailoverRecommendation captures a suggested score threshold for failover decisions.
type FailoverRecommendation struct {
	SuggestedScore float64
	Percentile     int
	SampleSize     int
	Confidence     float64
}

// RecommendFailover computes a score threshold derived from the current health distribution.
func (m *Monitor) RecommendFailover() FailoverRecommendation {
	m.mu.RLock()
	states := make([]*indexerState, 0, len(m.indexers))
	for _, state := range m.indexers {
		states = append(states, state)
	}
	m.mu.RUnlock()

	scores := make([]float64, 0, len(states))
	for _, state := range states {
		snap := state.snapshot(m.cfg)
		if snap.count < m.cfg.MinObservationsForRecommendation {
			continue
		}
		scores = append(scores, snap.score)
	}

	recommendation := FailoverRecommendation{
		SuggestedScore: m.cfg.HealthyScore,
		Percentile:     20,
		SampleSize:     len(scores),
		Confidence:     0,
	}

	total := len(states)
	if total > 0 {
		recommendation.Confidence = float64(len(scores)) / float64(total)
	}

	if len(scores) == 0 {
		return recommendation
	}

	sort.Float64s(scores)
	percentileIndex := int(math.Ceil(0.20*float64(len(scores)))) - 1
	if percentileIndex < 0 {
		percentileIndex = 0
	}
	threshold := scores[percentileIndex]
	if threshold < m.cfg.FailoverFloorScore {
		threshold = m.cfg.FailoverFloorScore
	}
	if threshold > m.cfg.HealthyScore {
		threshold = m.cfg.HealthyScore
	}
	recommendation.SuggestedScore = math.Round(threshold*10) / 10
	return recommendation
}

func (m *Monitor) ensureState(name string, discoveredAt time.Time) (*indexerState, bool) {
	m.mu.RLock()
	state, ok := m.indexers[name]
	m.mu.RUnlock()
	if ok {
		return state, false
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if state, ok = m.indexers[name]; ok {
		return state, false
	}

	state = newIndexerState(name, discoveredAt, m.cfg.SampleWindow)
	m.indexers[name] = state
	return state, true
}

type indexerState struct {
	name         string
	mu           sync.RWMutex
	history      []observation
	successes    int
	failures     int
	timeouts     int
	latencySum   time.Duration
	lastObserved time.Time
	lastError    string
	discoveredAt time.Time
	metadata     Metadata
}

type observation struct {
	outcome   Outcome
	latency   time.Duration
	timestamp time.Time
}

func newIndexerState(name string, discoveredAt time.Time, window int) *indexerState {
	return &indexerState{
		name:         name,
		history:      make([]observation, 0, window),
		discoveredAt: discoveredAt,
	}
}

func (s *indexerState) record(obs Observation, cfg Config) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.history = append(s.history, observation{outcome: obs.Outcome, latency: obs.Latency, timestamp: obs.Timestamp})
	switch obs.Outcome {
	case OutcomeSuccess:
		s.successes++
		s.latencySum += obs.Latency
	case OutcomeFailure:
		s.failures++
	case OutcomeTimeout:
		s.timeouts++
	}
	if obs.Error != nil {
		s.lastError = obs.Error.Error()
	} else if obs.Outcome == OutcomeSuccess {
		s.lastError = ""
	}
	s.lastObserved = obs.Timestamp

	if len(s.history) > cfg.SampleWindow {
		oldest := s.history[0]
		s.history = s.history[1:]
		switch oldest.outcome {
		case OutcomeSuccess:
			if s.successes > 0 {
				s.successes--
			}
			if s.latencySum > oldest.latency {
				s.latencySum -= oldest.latency
			} else {
				s.latencySum = 0
			}
		case OutcomeFailure:
			if s.failures > 0 {
				s.failures--
			}
		case OutcomeTimeout:
			if s.timeouts > 0 {
				s.timeouts--
			}
		}
	}
}

func (s *indexerState) updateMetadata(meta Metadata) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if meta.Tags == nil && s.metadata.Tags != nil {
		meta.Tags = s.metadata.Tags
	}
	s.metadata = meta
}

type stateSnapshot struct {
	score          float64
	successRate    float64
	failureRate    float64
	timeoutRate    float64
	averageLatency time.Duration
	count          int
	lastObserved   time.Time
	lastError      string
	discoveredAt   time.Time
	metadata       Metadata
}

func (s *indexerState) snapshot(cfg Config) stateSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := len(s.history)
	if total == 0 {
		return stateSnapshot{
			score:          cfg.HealthyScore,
			successRate:    1,
			failureRate:    0,
			timeoutRate:    0,
			averageLatency: 0,
			count:          0,
			lastObserved:   s.lastObserved,
			lastError:      s.lastError,
			discoveredAt:   s.discoveredAt,
			metadata:       s.metadata,
		}
	}

	successRate := 0.0
	failureRate := 0.0
	timeoutRate := 0.0
	if total > 0 {
		successRate = float64(s.successes) / float64(total)
		failureRate = float64(s.failures) / float64(total)
		timeoutRate = float64(s.timeouts) / float64(total)
	}
	averageLatency := time.Duration(0)
	if s.successes > 0 {
		averageLatency = time.Duration(int64(s.latencySum) / int64(s.successes))
	}

	score := successRate * 100
	score -= failureRate * cfg.FailurePenalty
	score -= timeoutRate * cfg.TimeoutPenalty
	if averageLatency > cfg.LatencyTarget {
		over := float64(averageLatency-cfg.LatencyTarget) / float64(cfg.LatencyTarget)
		if over > 1 {
			over = 1
		}
		score -= over * cfg.LatencyPenalty
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return stateSnapshot{
		score:          score,
		successRate:    successRate,
		failureRate:    failureRate,
		timeoutRate:    timeoutRate,
		averageLatency: averageLatency,
		count:          total,
		lastObserved:   s.lastObserved,
		lastError:      s.lastError,
		discoveredAt:   s.discoveredAt,
		metadata:       s.metadata,
	}
}
