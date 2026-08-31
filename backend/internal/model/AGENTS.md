# internal/model/ Working Conventions

Data structures shared across layers. See [`../../AGENTS.md`](../../AGENTS.md) for cross-layer invariants; see the "Catalog Production Data Model" and similar sections in [`../../README.md`](../../README.md) for field definitions.

## Boundaries

- Contains only plain data structures (structs with explicit `gorm:"column:...;primaryKey;type:..."` tags and a `TableName()` method); do not rely on GORM's implicit naming conventions.
- Must not contain business logic, validation rules, or behavioral methods (qualification checks, state transitions, etc. all stay in `internal/service`); model only describes data shape, never behavior.
- Field types, constraints, and nullability must match the schema tables in the corresponding README section; update the README before adding a field, so model, migration, and README never drift apart.
- For technical tables keyed by a natural key rather than a UUID (e.g. `seed_history`), add a comment next to the struct explaining why, following the existing pattern in `seed.go`.
- `handler`'s API DTOs and the models here are two separate sets of types (see [`../handler/AGENTS.md`](../handler/AGENTS.md)); a handler must never serialize a model directly.
