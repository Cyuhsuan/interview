# internal/service/ Working Conventions

The business logic layer. See [`../../AGENTS.md`](../../AGENTS.md) for cross-layer invariants; see [`../../README.md`](../../README.md) for business rules.

## Boundaries

- Must not import Gin or GORM, must not hold a `*gorm.DB`, and must not import concrete types from `internal/repository`.
- Service qualification, duration, availability, state transitions, and booking-legality decisions all happen at this layer — this is where README's "AI can only extract intent... legality must be decided by the deterministic service layer" rule is enforced.
- Follows the **ports** convention: each service declares the repository interfaces it needs (e.g. `ServiceRepository`, `ProfessionalRepository`), implemented by `internal/repository`; a service depends only on the interface it declared, never on a repository's concrete struct. When adding a new repository dependency, define the interface in the service package first, then have the corresponding repository provide an implementation with a `var _ Interface = (*Repository)(nil)` compile-time assertion.
- When a transaction boundary is needed, use the `Repository.WithTx(ctx, fn func(TxRepository) error) error` pattern (see `internal/service/seed`); do not assemble transactions outside the service.
- Errors always use named sentinel errors (e.g. `ErrInvalidServiceCode`) or typed errors implementing `Unwrap()`, so handlers can use `errors.Is`/`errors.As`; never return a bare string or an unclassified `fmt.Errorf`.
- Constants (regex patterns, limits, etc.) are cross-referenced in comments against the corresponding README section, so the code and the contract don't drift apart independently.
