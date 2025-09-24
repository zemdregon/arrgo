package contract

import (
	"context"
	"errors"
	"testing"

	"github.com/arrgo/arrgo/sdk/downloadclient"
)

type fakeClient struct {
	name      string
	priority  int
	supported map[string]bool

	healthErr  error
	enqueueErr error
}

func (f *fakeClient) Name() string { return f.name }

func (f *fakeClient) Priority() int { return f.priority }

func (f *fakeClient) Supports(req downloadclient.Request) bool {
	if f.supported == nil {
		return true
	}
	return f.supported[req.Protocol]
}

func (f *fakeClient) CheckHealth(ctx context.Context) error {
	_ = ctx
	return f.healthErr
}

func (f *fakeClient) Enqueue(ctx context.Context, req downloadclient.Request) error {
	_ = ctx
	_ = req
	return f.enqueueErr
}

func TestRunContract(t *testing.T) {
	suite := Suite{
		NewClient: func(t *testing.T) downloadclient.Client {
			t.Helper()
			return &fakeClient{
				name:     "example",
				priority: 5,
				supported: map[string]bool{
					"torrent": true,
				},
			}
		},
		SupportedRequests:   []downloadclient.Request{{Protocol: "torrent"}},
		UnsupportedRequests: []downloadclient.Request{{Protocol: "usenet"}},
		HealthCases: []HealthCase{
			{
				Name: "healthy",
			},
			{
				Name: "unhealthy",
				Setup: func(t *testing.T, client downloadclient.Client) {
					t.Helper()
					client.(*fakeClient).healthErr = errors.New("unhealthy")
				},
				Assert: AssertError,
			},
		},
		EnqueueCases: []EnqueueCase{
			{
				Name:    "ok",
				Request: downloadclient.Request{Protocol: "torrent"},
			},
			{
				Name:    "transient",
				Request: downloadclient.Request{Protocol: "torrent"},
				Setup: func(t *testing.T, client downloadclient.Client) {
					t.Helper()
					client.(*fakeClient).enqueueErr = downloadclient.ErrClientUnavailable
				},
				Assert: AssertErrorIs(downloadclient.ErrClientUnavailable),
			},
		},
	}

	Run(t, suite)
}

func TestRunContractWithCustomContext(t *testing.T) {
	suite := Suite{
		NewClient: func(t *testing.T) downloadclient.Client {
			t.Helper()
			return &fakeClient{name: "ctx", priority: 1}
		},
		HealthCases: []HealthCase{
			{
				Name: "canceled",
				Context: func() context.Context {
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					return ctx
				},
				Assert: func(t *testing.T, err error) {
					t.Helper()
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("expected context canceled, got %v", err)
					}
				},
				Setup: func(t *testing.T, client downloadclient.Client) {
					t.Helper()
					client.(*fakeClient).healthErr = context.Canceled
				},
			},
		},
	}

	Run(t, suite)
}
