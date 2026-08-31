# internal/platform/ Working Conventions

Cross-layer infrastructure (`config`, `database`, `httpproblem`, `httpserver`, `idgen`). See [`../../AGENTS.md`](../../AGENTS.md) for cross-layer invariants; see the "Tech Stack" section of [`../../README.md`](../../README.md) for stack boundaries.

## Boundaries

- `database`: the only place that creates a `*gorm.DB`; it manages the singleton connection pool via the Fx lifecycle (`OnStart` ping, `OnStop` close). No other package (including `internal/repository`) may create a second connection pool — they may only accept it via injection.
- `config`: `Load()` is the single entry point for reading all environment variables; if `CLINIC_TIMEZONE` (must pass `time.LoadLocation` validation) or `DATABASE_URL` is missing or invalid, it must return an error that fails startup — never fall back to an implicit default. `godotenv.Load()` may only be called when `APP_ENV != "production"`.
- `httpproblem`: every error response must go through `Write`/`WriteInternal` here, matching README's "Error Contract" format. `WriteInternal` must never leak `err.Error()` into the response body; when adding a new error-code constant, check it against README's HTTP-status mapping table — never invent a code that isn't listed in README.
- `httpserver`: `NewEngine()`/`RegisterServer()` only assemble the Gin engine and bind listen/shutdown to the Fx lifecycle; they mount no business routes — route registration is left to each module's `internal/handler/*/RegisterRoutes`.
- `idgen`: the sole entry point for generating every entity ID; the underlying source must be a CSPRNG (e.g. `google/uuid`'s `NewRandom()`); no other package may generate UUIDs independently or use a non-CSPRNG source.
- Each subpackage in this directory has a single responsibility and must not import another subpackage's internal details; when sharing across subpackages, pass state explicitly through `config.Config`.
