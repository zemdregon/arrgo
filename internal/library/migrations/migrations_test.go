package migrations

import "testing"

func TestPlanMigrationsRespectsAppliedVersions(t *testing.T) {
	applied := map[int]struct{}{1: {}, 2: {}, 3: {}}
	pending := planMigrations(registeredMigrations, applied, Options{})
	if len(pending) != 0 {
		t.Fatalf("expected no pending migrations, got %d", len(pending))
	}
}

func TestPlanMigrationsIncludesOptionalWhenEnabled(t *testing.T) {
	applied := map[int]struct{}{}
	pending := planMigrations(registeredMigrations, applied, Options{SeedDemoData: true})
	if len(pending) != 3 {
		t.Fatalf("expected 3 pending migrations, got %d", len(pending))
	}
	if pending[0].Version != 1 {
		t.Fatalf("expected migration 1 first, got %d", pending[0].Version)
	}
	if pending[1].Version != 2 {
		t.Fatalf("expected migration 2 second, got %d", pending[1].Version)
	}
	if pending[2].Version != 3 {
		t.Fatalf("expected migration 3 third, got %d", pending[2].Version)
	}
}

func TestPlanMigrationsSkipsSeedWhenDisabled(t *testing.T) {
	applied := map[int]struct{}{}
	pending := planMigrations(registeredMigrations, applied, Options{SeedDemoData: false})
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending migrations, got %d", len(pending))
	}
	if pending[0].Version != 1 {
		t.Fatalf("expected migration 1 first, got %d", pending[0].Version)
	}
	if pending[1].Version != 3 {
		t.Fatalf("expected migration 3 second, got %d", pending[1].Version)
	}
}
