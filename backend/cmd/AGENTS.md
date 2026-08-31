# cmd/ Working Conventions

Composition root. See [`../AGENTS.md`](../AGENTS.md) for cross-layer invariants; see [`../README.md`](../README.md) for architecture and contract.

## Boundaries

- `go.uber.org/fx` may only be used here (`cmd/api`, `cmd/calendar-worker`); business packages under `internal/` must not import Fx. When wiring up a new module, provide it in the corresponding `cmd/*/main.go`'s `fx.Provide` in the order repository → interface adapter → service → handler, following the existing pattern in `cmd/api/main.go`; `fx.Invoke` is only used to force construction of singletons (e.g. pinging the DB pool) and to register routes/the server.
- `cmd/migrate` is a standalone CLI that does not go through Fx or `internal/platform/httpserver`; it opens its own `gorm.Open` connection because migration/seeding is a one-off operational action and must not share the API process's DB pool lifecycle.
- Migrations and seeds must never be invoked automatically on `cmd/api` startup; they may only be run explicitly via `cmd/migrate`.
- Production must not depend on `godotenv`; if a required environment variable is missing, `cmd/api` must fail to start rather than fall back to an implicit default.
