package indexer

import (
	"context"
	"errors"
	"math"
	"sort"
	"testing"
	"time"
)

type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	return f.now
}

type fakeDiscoverer struct {
	metas []Metadata
	err   error
}

func (f fakeDiscoverer) DiscoverIndexers(_ context.Context) ([]Metadata, error) {
	if f.err != nil {
		return nil, f.err
	}
	// return copies to avoid mutation coupling
	result := make([]Metadata, len(f.metas))
	copy(result, f.metas)
	return result, nil
}

func TestMonitorRecordAndDiscovery(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1700000000, 0).UTC()}
	monitor := NewMonitor(Config{Clock: clock, SampleWindow: 10})

	created, err := monitor.Record(Observation{Indexer: "Alpha", Outcome: OutcomeSuccess, Latency: 250 * time.Millisecond})
	if err != nil {
		t.Fatalf("record observation: %v", err)
	}
	if !created {
		t.Fatalf("expected new indexer discovery")
	}

	clock.now = clock.now.Add(2 * time.Second)
	created, err = monitor.Record(Observation{Indexer: "Alpha", Outcome: OutcomeFailure, Error: errors.New("bad gateway")})
	if err != nil {
		t.Fatalf("record observation: %v", err)
	}
	if created {
		t.Fatalf("expected existing indexer")
	}

	reports := monitor.Snapshot()
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	report := reports[0]
	if report.Name != "Alpha" {
		t.Fatalf("unexpected report name: %s", report.Name)
	}
	if report.Observations != 2 {
		t.Fatalf("expected 2 observations, got %d", report.Observations)
	}
	if report.LastError != "bad gateway" {
		t.Fatalf("unexpected last error: %s", report.LastError)
	}
	if report.LastObserved != clock.now {
		t.Fatalf("unexpected last observed timestamp: %v", report.LastObserved)
	}
}

func TestMonitorScorePenalties(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1700000000, 0).UTC()}
	cfg := Config{
		Clock:          clock,
		SampleWindow:   20,
		FailurePenalty: 40,
		TimeoutPenalty: 70,
		LatencyPenalty: 15,
		LatencyTarget:  500 * time.Millisecond,
		HealthyScore:   80,
		DegradedScore:  60,
	}
	monitor := NewMonitor(cfg)

	for i := 0; i < 5; i++ {
		if _, err := monitor.Record(Observation{Indexer: "Alpha", Outcome: OutcomeSuccess, Latency: 400 * time.Millisecond}); err != nil {
			t.Fatalf("record success %d: %v", i, err)
		}
	}
	reports := monitor.Snapshot()
	if got := reports[0].Score; got != 100 {
		t.Fatalf("expected perfect score, got %.2f", got)
	}

	if _, err := monitor.Record(Observation{Indexer: "Alpha", Outcome: OutcomeFailure}); err != nil {
		t.Fatalf("record failure: %v", err)
	}
	if _, err := monitor.Record(Observation{Indexer: "Alpha", Outcome: OutcomeTimeout}); err != nil {
		t.Fatalf("record timeout: %v", err)
	}
	if _, err := monitor.Record(Observation{Indexer: "Alpha", Outcome: OutcomeSuccess, Latency: 3 * time.Second}); err != nil {
		t.Fatalf("record slow success: %v", err)
	}

	reports = monitor.Snapshot()
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	got := reports[0].Score
	want := 51.25
	if diff := math.Abs(got - want); diff > 0.01 {
		t.Fatalf("unexpected score: got %.2f want %.2f (diff=%.2f)", got, want, diff)
	}
	if reports[0].Healthy {
		t.Fatalf("expected unhealthy score")
	}
}

func TestMonitorSampleWindow(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1700000000, 0).UTC()}
	monitor := NewMonitor(Config{Clock: clock, SampleWindow: 3})

	outcomes := []Outcome{OutcomeSuccess, OutcomeFailure, OutcomeTimeout, OutcomeSuccess}
	for _, outcome := range outcomes {
		if _, err := monitor.Record(Observation{Indexer: "Alpha", Outcome: outcome}); err != nil {
			t.Fatalf("record outcome %s: %v", outcome, err)
		}
	}

	reports := monitor.Snapshot()
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].Observations != 3 {
		t.Fatalf("expected 3 retained observations, got %d", reports[0].Observations)
	}
	// The oldest success should have been evicted, leaving one success, one failure, one timeout.
	if got := reports[0].Availability; math.Abs(got-(1.0/3.0)) > 1e-9 {
		t.Fatalf("unexpected availability: %.6f", got)
	}
	if reports[0].FailureRate != 1.0/3.0 {
		t.Fatalf("unexpected failure rate: %.6f", reports[0].FailureRate)
	}
	if reports[0].TimeoutRate != 1.0/3.0 {
		t.Fatalf("unexpected timeout rate: %.6f", reports[0].TimeoutRate)
	}
}

func TestMonitorRecommendation(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1700000000, 0).UTC()}
	cfg := Config{
		Clock:                            clock,
		SampleWindow:                     10,
		HealthyScore:                     80,
		DegradedScore:                    60,
		FailoverFloorScore:               60,
		MinObservationsForRecommendation: 5,
	}
	monitor := NewMonitor(cfg)

	recordMany := func(name string, outcomes []Outcome) {
		for _, outcome := range outcomes {
			obs := Observation{Indexer: name, Outcome: outcome, Latency: 400 * time.Millisecond}
			if outcome == OutcomeTimeout {
				obs.Latency = 5 * time.Second
			}
			if _, err := monitor.Record(obs); err != nil {
				t.Fatalf("record outcome for %s: %v", name, err)
			}
		}
	}

	recordMany("A", []Outcome{OutcomeSuccess, OutcomeSuccess, OutcomeSuccess, OutcomeSuccess, OutcomeSuccess})
	recordMany("B", []Outcome{OutcomeSuccess, OutcomeSuccess, OutcomeSuccess, OutcomeSuccess, OutcomeFailure})
	recordMany("C", []Outcome{OutcomeSuccess, OutcomeTimeout, OutcomeSuccess, OutcomeTimeout, OutcomeSuccess})
	recordMany("D", []Outcome{OutcomeSuccess, OutcomeSuccess, OutcomeSuccess, OutcomeSuccess, OutcomeSuccess})
	recordMany("E", []Outcome{OutcomeSuccess, OutcomeFailure, OutcomeSuccess, OutcomeFailure, OutcomeSuccess})

	recommendation := monitor.RecommendFailover()
	if recommendation.SuggestedScore < 60 || recommendation.SuggestedScore > 80 {
		t.Fatalf("unexpected suggested score: %.2f", recommendation.SuggestedScore)
	}
	if recommendation.SampleSize != 5 {
		t.Fatalf("expected sample size 5, got %d", recommendation.SampleSize)
	}
	if recommendation.Confidence != 1 {
		t.Fatalf("expected confidence 1, got %.2f", recommendation.Confidence)
	}
}

func TestMonitorDiscoveryIntegration(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1700000000, 0).UTC()}
	discoverer := fakeDiscoverer{metas: []Metadata{{Name: "Alpha", Endpoint: "https://alpha.invalid", Categories: []string{"tv"}}, {Name: "Beta", Endpoint: "https://beta.invalid"}}}
	monitor := NewMonitor(Config{Clock: clock, Discoverers: []Discoverer{discoverer}})

	metas, err := monitor.Discover(context.Background())
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(metas) != 2 {
		t.Fatalf("expected 2 metadata entries, got %d", len(metas))
	}
	names := []string{metas[0].Name, metas[1].Name}
	sort.Strings(names)
	if names[0] != "Alpha" || names[1] != "Beta" {
		t.Fatalf("unexpected discovery names: %v", names)
	}

	reports := monitor.Snapshot()
	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}
	reportNames := []string{reports[0].Name, reports[1].Name}
	sort.Strings(reportNames)
	if reportNames[0] != "Alpha" || reportNames[1] != "Beta" {
		t.Fatalf("unexpected report names: %v", reportNames)
	}
	for _, report := range reports {
		if report.Observations != 0 {
			t.Fatalf("expected zero observations, got %d", report.Observations)
		}
		if !report.Healthy {
			t.Fatalf("expected neutral healthy status for %s", report.Name)
		}
	}
}
