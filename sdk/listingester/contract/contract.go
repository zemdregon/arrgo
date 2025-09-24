package contract

import (
	"errors"
	"fmt"
	"testing"

	"github.com/arrgo/arrgo/sdk/listingester"
)

// Suite describes the contract scenarios exercised against a provider.
type Suite struct {
	// NewProvider constructs a fresh provider instance for each scenario.
	NewProvider func(t *testing.T) listingester.Provider
	// SupportedRequests enumerates list requests that must be reported as supported.
	SupportedRequests []listingester.ListRequest
	// UnsupportedRequests enumerates list requests that must be reported as unsupported.
	UnsupportedRequests []listingester.ListRequest
	// HealthCases describe CheckHealth expectations.
	HealthCases []HealthCase
	// SyncCases describe Sync behaviour expectations.
	SyncCases []SyncCase
}

// HealthCase captures a CheckHealth contract scenario.
type HealthCase struct {
	Name   string
	Setup  func(t *testing.T, provider listingester.Provider)
	Assert func(t *testing.T, err error)
}

// SyncCase captures a Sync contract scenario.
type SyncCase struct {
	Name    string
	Request listingester.SyncRequest
	Setup   func(t *testing.T, provider listingester.Provider)
	Assert  func(t *testing.T, result *listingester.SyncResult, err error)
}

// Run executes the contract suite against the provider implementation.
func Run(t *testing.T, suite Suite) {
	t.Helper()

	if suite.NewProvider == nil {
		t.Fatal("contract: NewProvider is required")
	}

	provider := suite.NewProvider(t)
	if provider == nil {
		t.Fatal("contract: provider constructor returned nil")
	}

	for idx, req := range suite.SupportedRequests {
		req := req
		t.Run(fmt.Sprintf("supports/supported/%d", idx), func(t *testing.T) {
			t.Helper()
			if !provider.Supports(req) {
				t.Fatalf("provider does not support request: %+v", req)
			}
		})
	}

	for idx, req := range suite.UnsupportedRequests {
		req := req
		t.Run(fmt.Sprintf("supports/unsupported/%d", idx), func(t *testing.T) {
			t.Helper()
			if provider.Supports(req) {
				t.Fatalf("provider unexpectedly supports request: %+v", req)
			}
		})
	}

	for _, c := range suite.HealthCases {
		c := c
		name := c.Name
		if name == "" {
			name = "health"
		}
		t.Run(name, func(t *testing.T) {
			t.Helper()
			provider := suite.NewProvider(t)
			if c.Setup != nil {
				c.Setup(t, provider)
			}
			err := provider.CheckHealth(t.Context())
			if c.Assert != nil {
				c.Assert(t, err)
				return
			}
			if err != nil {
				t.Fatalf("check health failed: %v", err)
			}
		})
	}

	for _, c := range suite.SyncCases {
		c := c
		name := c.Name
		if name == "" {
			name = "sync"
		}
		t.Run(name, func(t *testing.T) {
			t.Helper()
			provider := suite.NewProvider(t)
			if c.Setup != nil {
				c.Setup(t, provider)
			}
			result, err := provider.Sync(t.Context(), c.Request)
			if c.Assert != nil {
				c.Assert(t, result, err)
				return
			}
			if err != nil {
				t.Fatalf("sync failed: %v", err)
			}
			if result == nil {
				t.Fatalf("sync returned nil result")
			}
		})
	}
}

// AssertErrorIs returns a helper that asserts the error matches a target.
func AssertErrorIs(target error) func(t *testing.T, err error) {
	return func(t *testing.T, err error) {
		t.Helper()
		if !errors.Is(err, target) {
			t.Fatalf("expected error %v, got %v", target, err)
		}
	}
}

// AssertNoError asserts that an error was not returned.
func AssertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// AssertSyncErrorIs returns a helper that asserts the sync error matches a target.
func AssertSyncErrorIs(target error) func(t *testing.T, _ *listingester.SyncResult, err error) {
	return func(t *testing.T, _ *listingester.SyncResult, err error) {
		t.Helper()
		if !errors.Is(err, target) {
			t.Fatalf("expected error %v, got %v", target, err)
		}
	}
}

// AssertSyncResult ensures the sync result is non-nil and satisfies the predicate.
func AssertSyncResult(assert func(t *testing.T, result *listingester.SyncResult)) func(t *testing.T, result *listingester.SyncResult, err error) {
	return func(t *testing.T, result *listingester.SyncResult, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("sync returned error: %v", err)
		}
		if result == nil {
			t.Fatalf("sync returned nil result")
		}
		if assert != nil {
			assert(t, result)
		}
	}
}
