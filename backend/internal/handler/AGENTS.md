# internal/handler/ Working Conventions

The HTTP translation layer. See [`../../AGENTS.md`](../../AGENTS.md) for cross-layer invariants; see [`../../README.md`](../../README.md) for the API contract.

## Boundaries

- May only import `gin-gonic/gin` and `internal/platform`; must not import GORM, hold a `*gorm.DB`, or call `internal/repository` directly. All data access goes through that module's `service`.
- `handler` and `repository` must not depend on each other directly; they must go through `service` (see [`../service/AGENTS.md`](../service/AGENTS.md)).
- Handlers only do request/response translation and input-format validation; they must not contain business rules (qualification, duration, availability, and state-transition decisions all stay in service).
- Responses use handler-private DTO structs with explicit `json` tags (camelCase); never return `internal/model` structs directly — model describes persistence-layer data, not the API contract.
- Sentinel errors returned by services are converted via `errors.Is` into `internal/platform/httpproblem` error responses; the `detail` field follows README's "Error Contract" and must never leak underlying error messages, SQL, or stack traces.
- Each module's (e.g. `catalog`) `RegisterRoutes(engine *gin.Engine, h *Handler)` mounts under the `/api/v1` group, following the naming used by the existing `catalog` and `health` modules.
