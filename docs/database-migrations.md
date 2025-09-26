# Database Migrations

> **Last updated:** 2025-03-10
> **Status:** Versioned migrations and demo seed data are available for the library service.

The arrgo master service now applies versioned PostgreSQL migrations at startup. This ensures contributors and operators share the
same schema regardless of deployment target and enables clean rollbacks when a release needs to be reverted.

## Migration Model

- Migrations are stored in `internal/library/migrations` and executed by `library.PostgresRepository.EnsureSchema`.
- Each migration defines `Up` and `Down` statements. `Up` applies schema changes; `Down` reverses them during rollback.
- Applied migrations are tracked in the `library_schema_migrations` table with the version number, name, and timestamp.
- The master process evaluates whether demo seed data should be inserted based on the `ARRGO_DATABASE_SEED_DEMO` environment variable.

## Demo Seed Data

When `ARRGO_DATABASE_SEED_DEMO` is `true`, the second migration loads sample catalog entities, library jobs, and metadata overrides.
This provides a realistic dataset for dashboards and API consumers in Docker Compose and other demo environments. Disable the flag in
production or automated tests to start with an empty database.

## Rollbacks

Use the helper in `internal/library/migrations` to roll back to a previous version when needed:

```go
ctx := context.Background()
pool, _ := pgxpool.New(ctx, os.Getenv("ARRGO_DATABASE_URL"))
defer pool.Close()

if err := migrations.Rollback(ctx, pool, 0); err != nil {
    log.Fatalf("rollback failed: %v", err)
}
```

Passing `0` reverts all migrations. Supplying a higher target version preserves migrations at or below the specified version. Rollbacks
execute each migration's `Down` statements inside a transaction and remove the entry from `library_schema_migrations`.

## Operational Guidance

- Always run `go test ./...` after editing migrations to ensure helper tests still pass.
- Version numbers must be unique and strictly increasing; use consecutive integers.
- Seed data should be idempotent (use `ON CONFLICT` clauses) so repeated startups remain safe.
- Production deployments should disable demo seed data and rely on explicit provisioning scripts or import pipelines.
