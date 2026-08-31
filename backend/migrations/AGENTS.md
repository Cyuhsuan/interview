# migrations/ Working Conventions

The schema-change process. See [`../AGENTS.md`](../AGENTS.md) for cross-layer invariants; see the "Catalog Production Data Model" section of [`../README.md`](../README.md) for schema definitions.

## Boundaries

- Uses `golang-migrate/migrate/v4`'s `{6-digit sequence}_{description}.{up|down}.sql` naming (e.g. `000001_create_catalog_tables.up.sql`); sequence numbers increase and are never reused, and `up`/`down` files must always come in pairs.
- Generated migration files must never be hand-edited after the fact; any further schema change always adds the next-numbered migration rather than modifying an existing file (an existing file may already have run in another environment).
- GORM `AutoMigrate` is forbidden; every schema change (constraints, indexes, defaults included) may only happen through the SQL migrations here.
- Migrations and seeds must never run automatically on `cmd/api` startup; they are only triggered explicitly via `cmd/migrate` (see [`../cmd/AGENTS.md`](../cmd/AGENTS.md)).
- Every migration must consider compatibility with existing data, a rollback/forward-fix path, lock duration, and compatibility with rolling deployment (old and new code running simultaneously); never assume there is no traffic at the moment of the change.
- Constraints (NOT NULL, CHECK, exclusion constraints, FK ON DELETE behavior) must be created at the SQL layer; they must never be enforced only by Go-level validation.
