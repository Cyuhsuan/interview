# Backend Working Conventions

This document applies to `backend/`; it is a cross-layer invariants document plus an index of layer-specific docs. Before implementing a specific feature, first resolve the corresponding open contract item in `backend/README.md`.

## Layer Documentation Index

The concrete rules for each layer's boundaries (who may import what, who owns which interface, who handles transactions) live in that layer's own `AGENTS.md` and are not repeated here. Read the relevant layer doc before touching that layer:

- [`cmd/AGENTS.md`](cmd/AGENTS.md) — composition root and CLI.
- [`internal/handler/AGENTS.md`](internal/handler/AGENTS.md) — HTTP translation layer.
- [`internal/service/AGENTS.md`](internal/service/AGENTS.md) — business logic layer.
- [`internal/repository/AGENTS.md`](internal/repository/AGENTS.md) — PostgreSQL access layer.
- [`internal/model/AGENTS.md`](internal/model/AGENTS.md) — shared data structures.
- [`internal/platform/AGENTS.md`](internal/platform/AGENTS.md) — cross-layer infrastructure.
- [`migrations/AGENTS.md`](migrations/AGENTS.md) — schema-change process.

## Contract

- `backend/README.md` is the single source of truth for backend architecture, data model, API, and external integrations, including the base path and versioning rules; this document and each layer's doc never duplicate endpoints or schemas.
- Public-behavior changes must be reflected in the README at the same time. Settings not yet approved by the clinic may only be listed under "Pending Clinic Confirmation" and must never be implemented with a default value.

## Cross-Layer Invariants

The following rules apply to every layer; violating them at any layer is a contract violation:

- PostgreSQL is the single source of truth for bookings, staff, services, availability, and audit state; Google Calendar and Microsoft Outlook are write-only external projections — no layer may let them influence availability or booking decisions in reverse. Overlap prevention, the state machine, and the sync flow follow README's "PostgreSQL-first Booking Consistency" and "Preventing Overlap and State"; external sync failures must never roll back or delete a committed booking.
- UUIDs, time, version, ETag, idempotency, status, and error codes follow the definitions in README's "Canonical Types"; no layer may invent its own format.
- Schema changes must use ordered, reviewable migrations, per the rules in [`migrations/AGENTS.md`](migrations/AGENTS.md).
- Calendar, AI, clock, ID-generator, and repository adapters must all have a contract test or a deterministic fake.
- The AI model may only understand language and extract candidate values; whether a booking is legal is always decided by deterministic code in `internal/service`.

## Verification

- Unit: qualifications, durations, business-hour boundaries, holidays, half-open intervals, timezone, and DST.
- PostgreSQL integration: overlap constraints, concurrent confirmation, idempotency, transactions, migrations, and the seeder.
- Adapter contract: throttling, timeout, token expiry, partial success, duplicate delivery, retry, `dead_letter`, and reconciliation.
- API: limits, ETag, error contract, authorization, CSRF/origin, rate limiting, and privacy.
- Before delivery, run the applicable format, static analysis, test, build, migration check, secret scan, and final diff review; never claim a check passed if it was not run.
