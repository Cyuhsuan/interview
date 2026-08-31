# internal/repository/ Working Conventions

The PostgreSQL access layer. See [`../../AGENTS.md`](../../AGENTS.md) for cross-layer invariants; see [`../../README.md`](../../README.md) for the data model.

## Boundaries

- This is the only place allowed to import GORM or hold a `*gorm.DB`. `*gorm.DB` must always be the singleton injected from `internal/platform/database`; a repository must never call `gorm.Open` itself or create a second connection pool (the standalone connection in `cmd/migrate` is the one exception — see [`../../cmd/AGENTS.md`](../../cmd/AGENTS.md)).
- Only implements the interfaces (ports) declared by the corresponding `internal/service` package (see [`../service/AGENTS.md`](../service/AGENTS.md)); must not contain business rules — qualification checks, duration calculation, and availability logic all belong to a different layer.
- `NewRepository(db *gorm.DB) *Repository` only accepts the existing singleton; it never creates a new connection. Use `var _ Interface = (*Repository)(nil)` for a compile-time interface assertion.
- Convert `gorm.ErrRecordNotFound` to `nil, nil` (not-found is not an error at this layer — the calling service decides how to handle it); wrap every other error with `fmt.Errorf("...: %w", err)` to preserve context.
- Modules that need transactions (e.g. `seed`) split into a `Repository` that owns `WithTx` and an unexported `txRepository` that only does CRUD, with no business rules.
- Query logic may only live here; `service` and `handler` must never operate on `*gorm.DB` or write SQL directly.
- Schema changes (new columns, indexes, constraints) always go through a migration, never `AutoMigrate` — see [`../../migrations/AGENTS.md`](../../migrations/AGENTS.md).
