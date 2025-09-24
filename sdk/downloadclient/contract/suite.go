package contract

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/arrgo/arrgo/sdk/downloadclient"
)

// Suite describes the contract tests that a download client adapter must satisfy.
type Suite struct {
	// NewClient must return a new client instance for each invocation.
	NewClient func(t *testing.T) downloadclient.Client

	// SupportedRequests contains sample requests that the adapter should accept.
	SupportedRequests []downloadclient.Request
	// UnsupportedRequests contains sample requests that the adapter should reject.
	UnsupportedRequests []downloadclient.Request

	// HealthCases allow adapters to prove health probe behaviour under different scenarios.
	HealthCases []HealthCase
	// EnqueueCases allow adapters to prove enqueue behaviour under different scenarios.
	EnqueueCases []EnqueueCase
}

// HealthCase defines a single health-check scenario.
type HealthCase struct {
	Name string
	// Setup can mutate the adapter prior to executing the health check.
	Setup func(t *testing.T, client downloadclient.Client)
	// Context overrides the default context (context.Background).
	Context func() context.Context
	// Assert validates the resulting error. Defaults to AssertNoError.
	Assert ErrorAssertion
}

// EnqueueCase defines a single enqueue scenario.
type EnqueueCase struct {
	Name string
	// Request identifies the work that should be enqueued in the scenario.
	Request downloadclient.Request
	// Setup can mutate the adapter prior to executing the enqueue call.
	Setup func(t *testing.T, client downloadclient.Client)
	// Context overrides the default context (context.Background).
	Context func() context.Context
	// Assert validates the resulting error. Defaults to AssertNoError.
	Assert ErrorAssertion
}

// ErrorAssertion validates the output of an adapter invocation.
type ErrorAssertion func(t *testing.T, err error)

// AssertNoError fails the test if err is not nil.
func AssertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// AssertError fails the test if err is nil.
func AssertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error, got nil")
	}
}

// AssertErrorIs constructs an ErrorAssertion that verifies errors.Is(err, target).
func AssertErrorIs(target error) ErrorAssertion {
	return func(t *testing.T, err error) {
		t.Helper()
		if !errors.Is(err, target) {
			t.Fatalf("expected error to be %v, got %v", target, err)
		}
	}
}

// AssertErrorContains verifies that the error message contains the provided substring.
func AssertErrorContains(substr string) ErrorAssertion {
	return func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("expected error containing %q, got nil", substr)
		}
		if !strings.Contains(err.Error(), substr) {
			t.Fatalf("expected error containing %q, got %v", substr, err)
		}
	}
}

// Run executes the contract tests defined by the suite.
func Run(t *testing.T, suite Suite) {
	t.Helper()

	if suite.NewClient == nil {
		t.Fatalf("contract: Suite.NewClient is required")
	}

	t.Run("metadata", func(t *testing.T) {
		client := suite.NewClient(t)
		if client == nil {
			t.Fatalf("contract: NewClient returned nil")
		}
		name := strings.TrimSpace(client.Name())
		if name == "" {
			t.Fatalf("contract: client.Name() must not be empty or whitespace-only")
		}
		if name != client.Name() {
			t.Fatalf("contract: client.Name() must be stable across calls")
		}
		firstPriority := client.Priority()
		if firstPriority != client.Priority() {
			t.Fatalf("contract: client.Priority() must be stable across calls")
		}
	})

	if len(suite.SupportedRequests) > 0 || len(suite.UnsupportedRequests) > 0 {
		t.Run("supports", func(t *testing.T) {
			client := suite.NewClient(t)
			for idx, req := range suite.SupportedRequests {
				if !client.Supports(req) {
					t.Fatalf("contract: supported request %d was rejected: %+v", idx, req)
				}
			}
			for idx, req := range suite.UnsupportedRequests {
				if client.Supports(req) {
					t.Fatalf("contract: unsupported request %d was accepted: %+v", idx, req)
				}
			}
		})
	}

	for idx, hc := range suite.HealthCases {
		hc := hc
		name := hc.Name
		if strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("case-%d", idx)
		}
		t.Run("health/"+name, func(t *testing.T) {
			client := suite.NewClient(t)
			if hc.Setup != nil {
				hc.Setup(t, client)
			}
			ctx := context.Background()
			if hc.Context != nil {
				ctx = hc.Context()
				if ctx == nil {
					t.Fatalf("contract: health case %q provided a nil context", name)
				}
			}
			err := client.CheckHealth(ctx)
			assertion := hc.Assert
			if assertion == nil {
				assertion = AssertNoError
			}
			assertion(t, err)
		})
	}

	for idx, ec := range suite.EnqueueCases {
		ec := ec
		name := ec.Name
		if strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("case-%d", idx)
		}
		t.Run("enqueue/"+name, func(t *testing.T) {
			client := suite.NewClient(t)
			if ec.Setup != nil {
				ec.Setup(t, client)
			}
			ctx := context.Background()
			if ec.Context != nil {
				ctx = ec.Context()
				if ctx == nil {
					t.Fatalf("contract: enqueue case %q provided a nil context", name)
				}
			}
			err := client.Enqueue(ctx, ec.Request)
			assertion := ec.Assert
			if assertion == nil {
				assertion = AssertNoError
			}
			assertion(t, err)
		})
	}
}
