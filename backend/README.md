# Backend Architecture & Contract

## Document Status

This document is the approved backend baseline contract. "Must" denotes a verifiable rule; product settings that have not yet been decided are collected under "Pending Clinic Confirmation" and must never be given an implicit default in production. Implementation-level contract gaps still to be resolved are collected under "Open Items Before Backend Implementation."

## Architectural Decisions

The backend is planned as a modular Go monolith backed by PostgreSQL, layered as handler / service / repository — not as DDD bounded contexts. PostgreSQL is the single source of truth for bookings, staff, services, availability, sync jobs, and audit state; Google Calendar and Microsoft Outlook are external projections that only receive data from PostgreSQL and must never influence availability or booking decisions in reverse.

### Tech Stack

| Purpose | Package | Scope of Use |
|---|---|---|
| HTTP router/middleware | [`gin-gonic/gin`](https://github.com/gin-gonic/gin) | Used only for HTTP server assembly in `internal/handler` and `internal/platform`; `service` and `repository` must not import Gin. |
| PostgreSQL ORM | [`gorm.io/gorm`](https://gorm.io/) | The single `*gorm.DB` connection pool is created by `internal/platform/database` and injected as a singleton via Fx; query logic may only live in `internal/repository` — `service` and `handler` must not import GORM or touch a `*gorm.DB`. `AutoMigrate` is forbidden; schema changes always go through the existing migration process. |
| Dependency injection | [`go.uber.org/fx`](https://github.com/uber-go/fx) | Used only in the `cmd/api` and `cmd/calendar-worker` composition roots to wire up handler/service/repository and platform dependencies; business packages must not import Fx. |
| Environment variable loading | [`joho/godotenv`](https://github.com/joho/godotenv) | Only loads `.env` locally/in development; production must supply configuration via real environment variables, and the API must fail to start if a required variable is missing — `.env` must never be a production configuration source. |

The AI may only extract intent and candidate field values. Service qualification, duration, state transitions, availability, and final booking decisions must all be handled by deterministic service-layer code.

### Directory Layout

```text
backend/
├── cmd/
│   ├── api/                    # HTTP API composition root
│   ├── calendar-worker/        # outbox delivery worker
│   └── migrate/                # migration/seed command
├── internal/
│   ├── handler/                 # HTTP request/response mapping, no business logic
│   │   ├── catalog/
│   │   ├── scheduling/
│   │   ├── booking/
│   │   └── conversation/
│   ├── service/                 # business rules, use cases, owned interfaces (ports)
│   │   ├── catalog/
│   │   ├── scheduling/
│   │   ├── booking/
│   │   └── conversation/
│   ├── repository/              # PostgreSQL data access, implements service-owned interfaces
│   │   ├── catalog/
│   │   ├── scheduling/
│   │   └── booking/
│   ├── calendar/                 # outbox delivery adapter and reconciliation
│   ├── model/                    # entities and value objects shared across layers
│   ├── platform/                 # HTTP server, PostgreSQL client, config, telemetry
│   └── shared/                   # approved shared value types only
├── migrations/
├── seeds/
├── test/
│   ├── contract/
│   └── integration/
└── README.md
```

`handler` is only responsible for HTTP request/response translation, input-format validation, and calling `service`; it must not touch SQL directly or call external SDKs. `service` holds the business logic and the interfaces for whatever external dependencies it needs (e.g. repository, calendar adapter), and must not import HTTP or SQL drivers. `repository` only implements the interfaces `service` defines and operates on PostgreSQL; it must not contain business rules. Feature modules communicate with each other only through exported service methods, requests/responses, or value objects — they never share a repository or ORM model; `shared` holds only types that are semantically identical across two or more modules.

| Module | Responsibility | Not Responsible For |
|---|---|---|
| Catalog | Services, staff, and explicit service qualifications | Availability and Calendar calls |
| Scheduling | Business rules, internal blocked slots, PostgreSQL appointments and availability | Creating appointments or reading external calendars |
| Booking | Session, final confirmation, overlap prevention, and the outbox transaction | Calling the Calendar SDK directly |
| Conversation | English conversation, scope boundaries, and AI candidate values | Approving bookings |
| Calendar | Outbox delivery, retry, and reconciliation | Changing appointment status |

Availability is the Scheduling service's responsibility; a Calendar event is an Appointment's external projection.

## Canonical Types

### ID

- Aggregate/entity IDs and foreign keys pointing at an entity always use UUID v4. Pure join tables may use a composite primary key made of UUID foreign keys; technical tables such as `seed_history` and idempotency records use the natural key explicitly defined in this document.
- UUIDs are generated by the application through an injectable, CSPRNG-backed `IDGenerator`; sequential integers, semantic slugs, and nil UUIDs are forbidden.
- PostgreSQL uses `uuid`; the API uses an RFC 9562 lowercase hyphenated string. Once created, an ID must never change, be reused, or be transferred.
- No separate `public_id` is created. `code` is an immutable business key, must never substitute for ID, and must match `^[A-Z][A-Z0-9_]{0,31}$`.

### Time and Intervals

- Production must configure a valid IANA clinic timezone; the API must not start without it, or with an invalid one.
- Instants are stored as PostgreSQL `timestamptz`, the database connection timezone is fixed to UTC, and precision is capped at microseconds.
- API instants use RFC 3339. Availability/appointment times must include the clinic's UTC offset at that time, and must also return the IANA `timeZone`.
- API `date` uses `YYYY-MM-DD`, interpreted in the clinic timezone. Intervals always use the half-open form `[start, end)`.
- `created_at` and `updated_at` are generated by PostgreSQL; `updated_at` only updates when data actually changes.

### Version and Idempotency

- Aggregate `version` uses a positive `bigint`, starting at `1`, incremented atomically by `1` on every persisted domain-state change.
- The API carries version via a strong ETag, formatted as `ETag: "3"`. Modifying a session and creating an appointment both require `If-Match`; if it is missing, return `428 PRECONDITION_REQUIRED`, and if it doesn't match, return `412 SESSION_VERSION_MISMATCH`. The body never repeats the version.
- `POST /appointments` must supply an `Idempotency-Key`: 16–128 ASCII characters, restricted to alphanumerics, `.`, `_`, `:`, and `-`.
- The idempotency scope is method + route + JWT `sub`, with a 24-hour retention. The same key with the same canonical request hash replays the original status/body; the same key with a different hash returns `409 IDEMPOTENCY_KEY_REUSED`. Concurrent requests with the same key may only ever produce one appointment.

## PostgreSQL-first Booking Consistency

The API request transaction only writes to PostgreSQL; external calendar writes must happen after commit.

1. Validate the session, `If-Match`, `Idempotency-Key`, and patient input.
2. Read business rules, internal blocked slots, and existing appointments only from PostgreSQL; never query Google or Microsoft busy intervals.
3. Re-validate availability and domain rules inside a single PostgreSQL transaction, creating the appointment, the idempotency record, the audit record, and one outbox record each for Google and Microsoft.
4. Once the commit succeeds, the appointment is immediately a confirmed internal fact; the API returns `201 Created` with `calendarDelivery=queued`.
5. The worker locks the outbox row inside a PostgreSQL transaction, marks it `processing`, and commits; it then writes to the external calendar using a stable idempotency key derived from the appointment ID and provider, and finally writes back the delivery result and event reference.
6. A failed external write must never roll back or delete the appointment; it must be retried, and once retries are exhausted it moves to `dead_letter` and alerts a human for manual handling.

### Preventing Overlap and State

- An appointment must store `professional_id`, `start_at`, `end_at`, and status, with `start_at < end_at`.
- PostgreSQL must use an exclusion constraint forbidding overlapping `confirmed` appointments for the same `professional_id` over `tstzrange(start_at, end_at, '[)')`. A constraint conflict maps to `409 SLOT_NO_LONGER_AVAILABLE`.
- BookingSession: `collecting`, `readyToConfirm`, `confirmed`, `expired`. Only `collecting → readyToConfirm`, `readyToConfirm → collecting|confirmed`, and any non-terminal state `→ expired` are allowed; `confirmed` and `expired` are terminal states.
- Appointment: version 1 only has `confirmed`; no cancellation transition is defined.
- Provider outbox: `pending`, `processing`, `retryable`, `delivered`, `dead_letter`.
- API `calendarDelivery`: `queued`, `partial`, `delivered`, `attentionRequired`. `delivered` only when both providers succeed; `partial` when one succeeds; `attentionRequired` when either reaches `dead_letter`.

## Catalog Production Data Model

Unless marked nullable, every column must be `NOT NULL`. Constraints must be created in PostgreSQL, not relied upon only through Go-level validation.

### `services`

| Column | Type | Constraint |
|---|---|---|
| `id` | `uuid` | Primary key |
| `code` | `varchar(32)` | Unique immutable business key |
| `display_name` | `varchar(100)` | 1–100 Unicode code points after trim |
| `duration_minutes` | `smallint` | `> 0` |
| `is_active` | `boolean` | Default `true` |
| `created_at`, `updated_at` | `timestamptz` | Default database current time |

### `professionals`

| Column | Type | Constraint |
|---|---|---|
| `id` | `uuid` | Primary key; used as the API ID |
| `code` | `varchar(32)` | Unique immutable business key |
| `display_name` | `varchar(100)` | 1–100 Unicode code points after trim; approved English name |
| `is_active` | `boolean` | Default `true`; when `false`, no new availability is generated |
| `created_at`, `updated_at` | `timestamptz` | Default database current time |

`Professional` does not store `level`. Service qualification always comes from an explicit association, never derived from a code or display name. A professional must never be hard-deleted; deactivation must not change existing appointments.

### `professional_service_qualifications`

| Column | Type | Constraint |
|---|---|---|
| `professional_id` | `uuid` | FK → `professionals.id ON DELETE RESTRICT` |
| `service_id` | `uuid` | FK → `services.id ON DELETE RESTRICT` |
| `created_at` | `timestamptz` | Default database current time |

Primary key is (`professional_id`, `service_id`). No duration or service-code array is stored. Before removing a qualification, confirm there are no related future appointments.

### `professional_calendars`

| Column | Type | Constraint |
|---|---|---|
| `id` | `uuid` | Primary key |
| `professional_id` | `uuid` | FK → `professionals.id ON DELETE RESTRICT` |
| `provider` | `varchar(16)` | Check: `google` or `microsoft` |
| `calendar_ref_ciphertext` | `bytea` | Application-layer encrypted reference |
| `encryption_key_id` | `varchar(128)` | Managed-key identifier; not key material |
| `is_active` | `boolean` | Default `true` |
| `verified_at` | `timestamptz` | Nullable |
| `created_at`, `updated_at` | `timestamptz` | Default database current time |

(`professional_id`, `provider`) must be unique. Before a professional can start accepting bookings, every active professional must have exactly one active, verified Google mapping and one active, verified Microsoft mapping. OAuth tokens, client secrets, and private keys must never be stored in a table, seed, or repository — they must live in a managed secret system. A mapping being temporarily unavailable, or the provider itself being down, does not affect PostgreSQL booking decisions; sync is handled by outbox retry and reconciliation.

### `seed_history`

| Column | Type | Constraint |
|---|---|---|
| `version` | `varchar(32)` | Primary key (natural key, not a UUID) |
| `checksum` | `varchar(64)` | SHA-256 hex digest, NOT NULL |
| `executed_at` | `timestamptz` | Default database current time |
| `executor_id` | `varchar(128)` | NOT NULL, records who executed it |

`checksum` uses `varchar` rather than `char`: `char(n)` pads the value with spaces to a fixed length, which throws off comparisons against the value read back due to trailing whitespace. No FK relationships — `seed_history` is a purely technical/audit table.

## Scheduling & Booking Production Data Model

Unless marked nullable, every column must be `NOT NULL`. Constraints must be created in PostgreSQL, not relied upon only through Go-level validation. Clinic business hours, holidays, and internal blocked slots are inputs to the "available time" decision, and per the root AGENTS.md's non-negotiable rules must live in PostgreSQL — never as environment variables or code constants; slot interval and minimum advance-booking time are computational parameters, and follow the same pattern as `CLINIC_TIMEZONE` in `internal/platform/config`: required environment variables with no default, and the API must fail to start if they're missing.

### `clinic_hours`

| Column | Type | Constraint |
|---|---|---|
| `day_of_week` | `smallint` | Primary key, `0` (Sunday) – `6` (Saturday) |
| `is_open` | `boolean` | NOT NULL |
| `open_time` | `time` | Nullable; must be `NULL` when `is_open = false` |
| `close_time` | `time` | Nullable; must be `NULL` when `is_open = false`, and must be `> open_time` when `is_open = true` |

Fixed at 7 rows (one per weekday). The table may be empty — until the clinic confirms actual hours, an empty table means "availability cannot be determined," and per the fail-closed rule, availability returns an empty result rather than assuming a default schedule.

### `clinic_closures`

| Column | Type | Constraint |
|---|---|---|
| `closure_date` | `date` | Primary key |
| `reason` | `varchar(200)` | Nullable |

Represents a full-day closure (e.g. a public holiday). Multi-day closures are represented as multiple rows; there is no range column.

### `professional_blocked_slots`

| Column | Type | Constraint |
|---|---|---|
| `id` | `uuid` | Primary key |
| `professional_id` | `uuid` | FK → `professionals.id ON DELETE RESTRICT` |
| `start_at`, `end_at` | `timestamptz` | NOT NULL, `start_at < end_at` |
| `reason` | `varchar(200)` | Nullable |
| `created_at` | `timestamptz` | Default database current time |

Represents an "internal blocked slot" (e.g. personal time off, administrative time). There is no corresponding API in version 1 — operations staff write directly to PostgreSQL. Availability calculations must exclude any candidate slot that overlaps a row in this table.

### `booking_sessions`

| Column | Type | Constraint |
|---|---|---|
| `id` | `uuid` | Primary key |
| `status` | `varchar(16)` | Check: `collecting`, `readyToConfirm`, `confirmed`, `expired` |
| `service_id` | `uuid` | Nullable, FK → `services.id ON DELETE RESTRICT` |
| `professional_id` | `uuid` | Nullable, FK → `professionals.id ON DELETE RESTRICT` |
| `slot_start_at`, `slot_end_at` | `timestamptz` | Nullable, appear as a pair, `slot_start_at < slot_end_at` |
| `slot_time_zone` | `varchar(64)` | Nullable, IANA name |
| `patient_name` | `varchar(100)` | Nullable, 1–100 Unicode code points after trim |
| `patient_email` | `varchar(254)` | Nullable, max 254 ASCII characters |
| `version` | `bigint` | NOT NULL, defaults to `1`, incremented atomically per "Version and Idempotency" under Canonical Types |
| `expires_at` | `timestamptz` | NOT NULL |
| `created_at`, `updated_at` | `timestamptz` | Default database current time |

State transitions follow the existing rules under "Preventing Overlap and State": only `collecting → readyToConfirm`, `readyToConfirm → collecting|confirmed`, and any non-terminal state `→ expired` are allowed; `confirmed` and `expired` are terminal states. `expires_at` is computed at creation time from the `BOOKING_SESSION_TTL_MINUTES` setting; every read/update operation must check `expires_at < now()` first, and any expired session is always treated as `expired` (`410 BOOKING_SESSION_EXPIRED`) — this must not depend on a background job having already flipped the `status` column.

### `appointments`

| Column | Type | Constraint |
|---|---|---|
| `id` | `uuid` | Primary key |
| `booking_session_id` | `uuid` | Unique, FK → `booking_sessions.id ON DELETE RESTRICT` |
| `service_id` | `uuid` | FK → `services.id ON DELETE RESTRICT` |
| `professional_id` | `uuid` | FK → `professionals.id ON DELETE RESTRICT` |
| `patient_name` | `varchar(100)` | NOT NULL |
| `patient_email` | `varchar(254)` | NOT NULL |
| `start_at`, `end_at` | `timestamptz` | NOT NULL, `start_at < end_at` |
| `time_zone` | `varchar(64)` | NOT NULL, IANA name |
| `status` | `varchar(16)` | Check: version 1 only has `confirmed` (no cancellation transition) |
| `created_at`, `updated_at` | `timestamptz` | Default database current time |

Must create `EXCLUDE USING gist (professional_id WITH =, tstzrange(start_at, end_at, '[)') WITH &&) WHERE (status = 'confirmed')` (requires `CREATE EXTENSION IF NOT EXISTS btree_gist`), implementing the existing "Preventing Overlap and State" rules; a constraint conflict maps to `409 SLOT_NO_LONGER_AVAILABLE`.

### `appointment_idempotency_keys`

| Column | Type | Constraint |
|---|---|---|
| `key` | `varchar(128)` | Primary key (natural key, not a UUID), 16–128 ASCII characters, restricted to alphanumerics, `.`, `_`, `:`, `-` |
| `request_hash` | `varchar(64)` | NOT NULL, SHA-256 hex digest of the canonical request |
| `appointment_id` | `uuid` | Nullable, FK → `appointments.id ON DELETE RESTRICT` |
| `response_status` | `smallint` | NOT NULL |
| `response_body` | `jsonb` | NOT NULL |
| `created_at` | `timestamptz` | Default database current time |

The scope is method + route + JWT `sub` (defined under Canonical Types); this table only fills in the schema. The same `key` with the same `request_hash` replays the stored `response_status`/`response_body`; the same `key` with a different `request_hash` returns `409 IDEMPOTENCY_KEY_REUSED`. Retention is 24 hours; an expiry-cleanup job is not yet implemented in this phase — this is a known limitation, see "Pending Clinic Confirmation."

### `appointment_audit_log`

| Column | Type | Constraint |
|---|---|---|
| `id` | `uuid` | Primary key |
| `entity_id` | `uuid` | NOT NULL |
| `action` | `varchar(64)` | NOT NULL |
| `actor_id` | `varchar(128)` | NOT NULL |
| `created_at` | `timestamptz` | Default database current time |

Stores only entity ID, action, actor ID, and timestamp, per the log/audit restrictions in "Security & Operations Baseline"; it must never store patient name, email, or message content.

### `appointment_outbox`

| Column | Type | Constraint |
|---|---|---|
| `id` | `uuid` | Primary key |
| `appointment_id` | `uuid` | FK → `appointments.id ON DELETE RESTRICT` |
| `provider` | `varchar(16)` | Check: `google` or `microsoft` |
| `status` | `varchar(16)` | Check: `pending`, `processing`, `retryable`, `delivered`, `dead_letter` |
| `idempotency_key` | `varchar(128)` | Unique; `appt:{appointmentId}:{provider}`, see step 5 of "PostgreSQL-first Booking Consistency" |
| `attempt_count` | `integer` | Default `0` |
| `next_attempt_at` | `timestamptz` | Default database current time |
| `event_reference` | `varchar(512)` | Nullable, the provider event reference returned by the adapter |
| `last_error` | `varchar(500)` | Nullable, sanitized failure reason, must never contain the provider's raw response body (see "Security & Operations Baseline") |
| `created_at`, `updated_at` | `timestamptz` | Default database current time |

(`appointment_id`, `provider`) must be unique — every appointment has exactly one outbox row per provider. `internal/service/booking` creates one `pending` row each for google and microsoft in the same transaction that confirms the appointment; `internal/service/calendar`/`cmd/calendar-worker` advance the remaining states per steps 5–6 of "PostgreSQL-first Booking Consistency," without ever changing appointment status.

### Outbox/Calendar Delivery (Implemented: outbox mechanism + sandbox adapter; real Google/Microsoft OAuth not yet wired up)

After `POST /appointments` succeeds, one `pending` `appointment_outbox` row is created each for google and microsoft in the same transaction, and the response includes `calendarDelivery=queued` (see the endpoint schema below). `cmd/calendar-worker` polls due rows, locks and marks them `processing` before committing, then calls `internal/service/calendar.Adapter.Create`; on success it writes back `delivered` and `event_reference`; on a transient failure it marks `retryable` and schedules `next_attempt_at` per `CALENDAR_OUTBOX_RETRY_BACKOFF_SECONDS` (exponential backoff); once it reaches `CALENDAR_OUTBOX_MAX_ATTEMPTS` or hits a permanent failure, it moves to `dead_letter`. `calendarDelivery` is computed live from the two outbox rows by `internal/service/calendar.Service.DeliveryStatus`, with no caching: `delivered` only when both are `delivered`; `partial` when one is `delivered`; `attentionRequired` when either is `dead_letter`; otherwise `queued`. A failed external write never rolls back or deletes an already-committed appointment, consistent with "PostgreSQL-first Booking Consistency."

The only `Adapter` implementation right now is `internal/calendar.SandboxAdapter`: a deterministic fake that never makes any network request, returning `sandbox:{provider}:{idempotencyKey}` as the event reference. This exists so the outbox schema, worker, retry/dead_letter state machine, and `calendarDelivery` mapping could be built and tested before the real OAuth authorization model is approved. **It is not a real Google or Microsoft Calendar integration, and must never be described as production-ready Calendar sync.** The `professional_calendars` table (see the schema above) has not yet been created — it is only needed once real provider credentials are wired up; the current sandbox worker neither queries nor depends on it. Real integration cannot begin until "Google/Microsoft authorization model, tenant permissions, and credential storage" (see item 2 under "Pending Clinic Confirmation") is approved: the credential-storage approach has already been provisionally decided as application-layer encryption into `professional_calendars.calendar_ref_ciphertext` (reusing the existing column design), but the actual OAuth flow, tenant permissions, and creation of that table are still pending sandbox validation and clinic approval.

Reconciliation currently only offers `internal/service/calendar.Service.Reconcile`, which returns the current `dead_letter` backlog for the caller to log/alert on; who actually receives alerts and the manual-handling SLA are still pending clinic confirmation (see item 3 under "Pending Clinic Confirmation"), so this deliberately does not integrate with any external notification system yet.

## Reference-data Seeder

The seeder is a permissioned, explicit operational command and must never run automatically at API startup. The table below must match the "Clinic Model" table in the root README exactly; it is listed here to give the seed artifact's precise insert values.

| Service code | Display name | Duration |
|---|---|---:|
| `A` | `Service A` | 60 minutes |
| `B` | `Service B` | 60 minutes |
| `C` | `Service C` | 150 minutes |
| `D` | `Service D` | 120 minutes |
| `E` | `Service E` | 360 minutes |

| Professional code | Display name | Qualifications |
|---|---|---|
| `JUNIOR` | `Junior` | A, B |
| `SENIOR_1` | `Senior 1` | A, B, C, D, E |
| `SENIOR_2` | `Senior 2` | A, B, C, D, E |

1. A seed artifact has an immutable version and a SHA-256 checksum; a successful run is recorded in `seed_history(version, checksum, executed_at, executor_id)`, with `version` as the primary key. If the same version/checksum already exists, it's a no-op success; if the same version exists with a different checksum, it fails.
2. Services and professionals are looked up by `code`. If missing, a UUID is generated and the row is inserted; if it already exists, the fixed display name, duration, and other static fields above are verified, never overwritten or re-activated — any discrepancy fails the run.
3. Only the missing rows among the 12 qualification pairs are inserted; extra professionals or qualifications are never auto-deleted. Changing the fixed data requires a new-version artifact.
4. All validation, inserts, and the `seed_history` write happen in a single PostgreSQL transaction; any conflict rolls back everything.
5. Calendar mappings, emails, demo IDs, and credentials must never appear in a seed.

Acceptance covers first run/re-run, checksum/static-field conflicts, rollback, FK/unique constraints, all 12 qualification combinations, and deactivated professionals.

## RESTful API Contract

Base path is `/api/v1`, HTTPS UTF-8 JSON only. Request body is capped at 64 KiB; name is 1–100 Unicode code points after trim, email is at most 254 ASCII characters, message is at most 2,000 Unicode code points.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/services` | Get active services |
| `GET` | `/professionals?serviceCode=C` | Get qualified, active professionals |
| `GET` | `/availability?serviceCode=C&date=2026-09-02` | Get anonymous time slots |
| `POST` | `/booking-sessions` | Create a short-lived session |
| `GET` | `/booking-sessions/{id}` | Get a session |
| `PATCH` | `/booking-sessions/{id}` | Update a session with `If-Match` |
| `POST` | `/booking-sessions/{id}/messages` | Send an English patient message |
| `POST` | `/voice/transcriptions` | Upload a recording, get back AI-transcribed text |
| `POST` | `/appointments` | Confirm a booking with `If-Match` and `Idempotency-Key` |

The appointment body contains only the UUID `bookingSessionId`, no version. A booking session's `selectedSlot` must contain a UUID `professionalId`, RFC 3339 `start`/`end`, and an IANA `timeZone`.

Patient identity is never verified through an account or login: while a booking session is `collecting`, it gathers the patient's name and email, used only as the contact method and calendar-invite recipient for that appointment. Any external email may be used to book, with no pre-registration, domain restriction, or ownership verification required; email is never used as a long-lived account identifier, and must never be used to query across sessions or associate a patient's booking history.

Public sessions must use a signed JWT delivered via a `Secure; HttpOnly; SameSite=Strict` cookie, never stored in `localStorage` or `sessionStorage`. The JWT must contain at least `iss`, `aud`, `sub`, `jti`, `iat`, `nbf`, and `exp`, and must never contain patient name, email, or message content; the server must pin the allowed signing algorithms and validate every claim. Cookie authentication must still be paired with a CSRF token and a non-empty exact-origin allowlist.

Rate-limit values come from environment configuration, and production must not start without them being set; exceeding the limit returns `429` with `Retry-After`. Version 1 does not provide an appointment list, cancellation, or rescheduling API.

### Error Contract

Errors use `application/problem+json` and must include `type`, `title`, `status`, `code`, `detail`, and a request-scoped `instance`; field-level errors may include `field` and `code` inside `errors[]`. `detail` must be safe, concise English, and must never contain a stack trace, SQL, patient data, tokens, or a provider response.

| HTTP | Code |
|---:|---|
| `400` | `INVALID_REQUEST` |
| `409` | `SLOT_NO_LONGER_AVAILABLE` / `IDEMPOTENCY_KEY_REUSED` |
| `410` | `BOOKING_SESSION_EXPIRED` |
| `412` | `SESSION_VERSION_MISMATCH` |
| `413` | `REQUEST_TOO_LARGE` |
| `422` | `VALIDATION_FAILED` |
| `428` | `PRECONDITION_REQUIRED` |
| `429` | `RATE_LIMITED` |
| `503` | `AVAILABILITY_UNAVAILABLE` / `VOICE_TRANSCRIPTION_UNAVAILABLE` |
| `500` | `INTERNAL_ERROR` |

### Catalog Endpoint Schemas

`GET /services` returns an array of active services:

```json
[
  {
    "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
    "code": "A",
    "displayName": "Service A",
    "durationMinutes": 60
  }
]
```

`GET /professionals?serviceCode=C` returns an array of professionals who are both active and qualified for that service:

```json
[
  {
    "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
    "code": "SENIOR_1",
    "displayName": "Senior 1"
  }
]
```

`serviceCode` is a required query parameter and must match `^[A-Z][A-Z0-9_]{0,31}$`; if missing, return `400 INVALID_REQUEST` (`errors[].field="serviceCode"`, `code="REQUIRED"`); if malformed, return the same status (`code="INVALID_FORMAT"`). If `serviceCode` is well-formed but no active matching service exists, or that service currently has no active qualified professionals, return `200` with an empty array `[]` — this is a valid result for a valid query, not an error.

### Scheduling/Booking Endpoint Schemas

`GET /availability?serviceCode=C&date=2026-09-02` returns anonymous candidate time slots computed from the `serviceCode`'s duration, qualified professionals, `clinic_hours`, `clinic_closures`, `professional_blocked_slots`, and existing `confirmed` appointments:

```json
[
  {
    "professionalId": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
    "start": "2026-09-02T09:00:00+08:00",
    "end": "2026-09-02T10:00:00+08:00",
    "timeZone": "Asia/Taipei"
  }
]
```

`serviceCode` follows the same rule as Catalog; `date` is a required `YYYY-MM-DD`, interpreted in `CLINIC_TIMEZONE`; a malformed value returns `400 INVALID_REQUEST` (`code="INVALID_FORMAT"`). The following cases return `200` with an empty array `[]` (a valid empty result, not an error): `serviceCode` has no qualified professionals, `clinic_hours.is_open=false` for that day, `clinic_hours` has no row yet for that weekday, the day is in `clinic_closures`, or no slots remain after excluding existing appointments and blocked slots. If PostgreSQL cannot be reached or the query fails, follow the fail-closed rule and return `503 AVAILABILITY_UNAVAILABLE` — never return a guessed result.

`POST /booking-sessions` may take an empty body `{}`; it creates a new session with `status="collecting"`, returning `201`, `ETag: "1"`, `Location: /api/v1/booking-sessions/{id}`, with the body being the session representation below.

`GET /booking-sessions/{id}` returns the current session:

```json
{
  "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
  "status": "collecting",
  "serviceCode": null,
  "selectedSlot": null,
  "patientName": null,
  "patientEmail": null,
  "expiresAt": "2026-08-27T10:15:00Z"
}
```

The `ETag` header carries the current `version`. If `id` doesn't exist or has expired (`expires_at < now()`), return `410 BOOKING_SESSION_EXPIRED`.

`PATCH /booking-sessions/{id}` (`If-Match` required; missing returns `428 PRECONDITION_REQUIRED`, and a value that doesn't match the current `version` returns `412 SESSION_VERSION_MISMATCH`) allows a partial update:

```json
{
  "serviceCode": "C",
  "selectedSlot": {
    "professionalId": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
    "start": "2026-09-02T09:00:00+08:00",
    "end": "2026-09-02T10:00:00+08:00",
    "timeZone": "Asia/Taipei"
  },
  "patientName": "Jane Doe",
  "patientEmail": "jane@example.com",
  "status": "readyToConfirm"
}
```

All fields are optional; only fields present in the request are updated. `status` only accepts an explicit transition target; an illegal transition (see the state machine under "Preventing Overlap and State") returns `422 VALIDATION_FAILED`. Before transitioning into `readyToConfirm`, the service layer must confirm that `serviceCode`, `selectedSlot`, `patientName`, and `patientEmail` are all filled in, and that the slot is still within the range `GET /availability` would return; otherwise return `422 VALIDATION_FAILED` with the missing fields marked in `errors[]`. On success, return `200` with the updated session representation, and `ETag` incremented to the new `version`.

`POST /appointments` (`If-Match` is the session's current `version`; `Idempotency-Key` is required, format per Canonical Types) has a body containing only `bookingSessionId`:

```json
{ "bookingSessionId": "3fa85f64-5717-4562-b3fc-2c963f66afa6" }
```

On success, within a single transaction it re-validates slot availability, creates an `appointments` row, an `appointment_idempotency_keys` row, an `appointment_audit_log` row, and one `appointment_outbox` pending row each for google and microsoft; the session transitions to `confirmed`. Returns `201`:

```json
{
  "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
  "serviceCode": "C",
  "professionalId": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
  "patientName": "Jane Doe",
  "patientEmail": "jane@example.com",
  "start": "2026-09-02T09:00:00+08:00",
  "end": "2026-09-02T10:00:00+08:00",
  "timeZone": "Asia/Taipei",
  "calendarDelivery": "queued"
}
```

`calendarDelivery` is always `queued` right after confirmation; a replay of the same `Idempotency-Key`/`request_hash` queries the current status of both outbox rows live and may return `queued`, `partial`, `delivered`, or `attentionRequired` (see the mapping rules in "Outbox/Calendar Delivery") — it is not a fixed value. If the session isn't in `readyToConfirm`, return `422 VALIDATION_FAILED`; if the slot has been taken (exclusion-constraint conflict), return `409 SLOT_NO_LONGER_AVAILABLE`; if `Idempotency-Key` is reused with a different request hash, return `409 IDEMPOTENCY_KEY_REUSED`; if reused with the same hash, replay the original status/body.

`POST /booking-sessions/{id}/messages` (`If-Match` is not required — this endpoint internally reads the session's current version and applies changes itself, without exposing optimistic locking externally) sends one English patient message:

```json
{ "message": "I'd like to book a cleaning next Tuesday afternoon" }
```

`message` is required, 1–2,000 Unicode code points after trim (same global message-length limit stated at the top of this section); missing or blank returns `400 INVALID_REQUEST` (`errors[].field="message"`, `code="REQUIRED"`), exceeding the limit returns the same status (`code="TOO_LONG"`). If the session doesn't exist or has expired, return `410 BOOKING_SESSION_EXPIRED`, consistent with the other session endpoints.

On success, returns `200` with the same session representation as `GET /booking-sessions/{id}`, plus three extra fields:

```json
{
  "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
  "status": "collecting",
  "serviceCode": "C",
  "selectedSlot": null,
  "patientName": null,
  "patientEmail": null,
  "expiresAt": "2026-08-27T10:15:00Z",
  "reply": "Got it — a Service C appointment. What date would you like to come in?",
  "offeredSlots": [],
  "outOfScope": false
}
```

- `reply`: the bot's reply text in English. **It is always assembled from a fixed, deterministic backend template** — AI is only used to decide which template applies this turn (e.g. "service extracted but date missing," or "classified as an out-of-scope emergency request"); the AI's freely generated text must never be returned directly as `reply`, so a model glitch can never produce a diagnosis, a price quote, or other out-of-scope content.
- `offeredSlots`: this turn's candidate time slots, computed by Scheduling (the same logic as `GET /availability`), shaped like `GET /availability`'s elements (`professionalId`/`start`/`end`/`timeZone`); it is non-empty only when the session already knows `serviceCode` and this turn's message resolves to an explicit date — otherwise it is always `[]`. This candidate list is also stored in the session's `offered_slots` (an internal field, never returned to the client), acting as **limited "previous turn" memory** — not full multi-turn history, only a reverse lookup into the immediately preceding turn's offer: if this turn doesn't supply a new `serviceCode`/date and instead uses an ordinal ("the first one") or a time period ("the morning one") to refer to one of them, `/messages` resolves the reference against the server's own sort of the candidate list (by `start` ascending, tie-broken by `professionalId` for determinism), and re-invokes `GetAvailability` live before applying it to confirm it's still bookable (fail-closed — the stored candidate is never trusted). Only if it's confirmed still bookable is `selectedSlot` written and `offered_slots` cleared; if it's been taken, it is not applied — the latest candidates are re-offered and the stale ones cleared. Ordinals only cover 1–5 (matching the candidate cap `maxOfferedSlots`) and "the last one"; exact times (e.g. "9am") are never parsed. When the reference can't be resolved or is out of scope, it is simply ignored and the normal flow for that day continues — this is never treated as an error. **Known limitation**: if the patient's frontend currently has a professional filter applied, the visually "first" option on screen may not match the server's own ordinal — this is not hidden, and is documented here for implementation and testing reference. Aside from this reference path, selecting a slot can always also be done through the existing `PATCH /booking-sessions/{id}` (with `selectedSlot`), and that behavior is unchanged.
- `outOfScope`: whether this turn's message was classified as out of scope (diagnosis, prescription, emergency, quote, insurance, or a cancel/reschedule request). When classified `true`, `reply` is always the fixed template for that category (directing the patient to contact the clinic directly, and to seek immediate care for an emergency), and this turn **does not** modify any session field, even if the message also happened to include service or date information.

The `ETag` header always carries the session's current version (regardless of whether this turn actually changed any field). If an AI candidate value doesn't map to a valid service code, or a candidate date can't be parsed, that is **not an error** — the service layer simply discards that candidate and `reply` becomes a clarifying question asking the patient to rephrase; the HTTP response is still `200`. Similarly, if the session's version was changed by another request just before this one applied its update, the service internally re-reads it once and retries once — still returning `200` (with `reply` asking the patient to say it again), never surfacing an optimistic-lock conflict as `412` to a chat user. The only cases that return something other than `200` are: the request itself is malformed (`400`); the session doesn't exist or has expired (`410`); the PostgreSQL query for `offeredSlots` fails (per the fail-closed rule, return `503 AVAILABILITY_UNAVAILABLE` — never a guessed slot list); or an unexpected server error (`500`).

## Calendar Adapter Contract

The provider-neutral port (`internal/service/calendar.Adapter`) is only responsible for projecting a PostgreSQL appointment into an external system. Version 1 supports `Create`, health checks, and retry classification (`internal/service/calendar.RetryableError` distinguishes transient from permanent failures) and reconciliation (`Service.Reconcile` reports the `dead_letter` backlog); it does not provide `Busy`, `Update`, or `Cancel`.

The only concrete implementation right now is `internal/calendar.SandboxAdapter` — a deterministic fake that makes no outbound requests, used to build and test the outbox/worker/retry state machine (see "Outbox/Calendar Delivery") before the real OAuth authorization model is approved. It is not a real integration with Google or Microsoft.

## AI Provider Adapter Contract

The provider-neutral port (`internal/service/conversation.AIProvider`) is only responsible for "single-turn message → candidate values + scope classification": given a patient message, a reference time, and the currently known list of valid service codes, it outputs candidate `serviceCode`/date/time-of-day preference (morning/afternoon/evening)/patient name/email/`offeredSlotOrdinal` (the patient's **positional** reference to the previous turn's offered slots, 1–5 or -1 for the last one, positional only — never an exact time), plus whether the message is out of scope. The port has no multi-turn memory and does not decide booking legality — that is always handled by `internal/service/conversation` calling into the existing deterministic `internal/service/booking` and `internal/service/scheduling` logic; `offeredSlotOrdinal` is likewise just a candidate value — whether it actually maps to a still-bookable slot is decided by `internal/service/conversation` after a live `GetAvailability` check; the AI itself never judges whether any slot is valid or still available.

Version 1 provides a single concrete implementation via `internal/ai`: an OpenAI-compatible Chat Completions HTTP client that requires the model to return its extraction as a fixed JSON schema. `AI_PROVIDER_API_KEY`, `AI_PROVIDER_BASE_URL`, and `AI_PROVIDER_MODEL` must be set at startup; if any is missing, `cmd/api` must fail to start immediately, following the same fail-closed convention as `CLINIC_TIMEZONE`/`DATABASE_URL` — never an implicit default. If a call times out or the response can't be parsed as the expected JSON schema, that counts as an extraction failure: the conversation service must fall back to a fixed clarifying reply, must never let the exception bubble up as a `500`, and must never apply any candidate value to the session.

**This is the development/test integration choice for this phase, not the approved production AI provider referenced in item 3 of the root README's "Decisions Required Before Implementation."** The formal provider selection and the applicable health-data/privacy agreements are still pending clinic approval, and until approved, this adapter's defaults must never be represented externally as a production commitment.

### Voice Transcription Endpoint

`POST /voice/transcriptions` (no session id or `If-Match` required — transcription itself is stateless; the client must still send the returned text into the existing `/booking-sessions/{id}/messages` for it to affect the booking session) accepts `multipart/form-data` with an `audio` field containing the recording:

- The request body is capped at **10 MiB** — this is an exception specific to this endpoint, not a relaxation of the global 64 KiB JSON body cap stated at the top of this section; exceeding it returns `413 REQUEST_TOO_LARGE`.
- `audio`'s `Content-Type` only accepts the whitelist `audio/webm`, `audio/ogg`, `audio/mp4`, `audio/wav`, `audio/mpeg`; anything outside the whitelist, or a missing file, returns `400 INVALID_REQUEST`.
- On success, returns `200`:

  ```json
  { "text": "I'd like to book a cleaning next Tuesday afternoon" }
  ```

- If the underlying AI transcription provider times out or fails, return `503 VOICE_TRANSCRIPTION_UNAVAILABLE` — unlike the §AI Provider Adapter Contract's silent fallback to a clarifying template on extraction failure, this endpoint's only output is the text itself, so there's no template to substitute on failure; hence it explicitly returns 503 so the frontend can fall back to text input, which remains fully available at all times.
- The underlying implementation reuses `internal/ai`'s existing OpenAI-compatible provider (the same `AI_PROVIDER_BASE_URL`/`AI_PROVIDER_API_KEY`), plus a required `AI_PROVIDER_TRANSCRIPTION_MODEL` environment variable; if missing, `cmd/api` fails to start immediately, following the same pattern as the other AI/clinic settings — never an implicit default. Transcription requests always specify `language=en`, since the patient-facing side only supports English. **This is likewise a development/test integration choice for this phase, not an approved production provider**; the health-data/privacy agreements involved for audio data mirror those described in §AI Provider Adapter Contract, and are still pending clinic approval.

The Google and Microsoft authorization models must each be validated in sandbox and approved separately; they are not assumed to share the same OAuth flow. Each provider must use least-privilege access, and credential owner, rotation, revocation, reauthorization, and tenant/calendar access boundaries must all be documented.

## Security & Operations Baseline

- Production must have TLS, managed secrets, encrypted storage/backups, point-in-time recovery, staff SSO/RBAC, and an audit trail.
- Logs, traces, metrics, and alerts must never contain patient messages, name, email, tokens, raw calendar references, or provider response bodies. Audit records only use entity ID, action, actor ID, and timestamp.
- The applicable privacy/health-data review and vendor agreement must be completed before handling patient data.
- Monitor the outbox backlog, `dead_letter`, provider health, and booking conflicts daily; run access reviews, backup restore tests, and provider outage drills quarterly.

## Pending Clinic Confirmation

1. Clinic IANA timezone and break periods. Business hours and holidays already have the `clinic_hours`/`clinic_closures` schema in place (see "Scheduling & Booking Production Data Model"), but the actual values are still pending from the clinic; the tables may be empty, in which case, per the fail-closed rule, availability is treated as none — never assume a default schedule. Slot interval and minimum advance-booking time use required environment variables; the actual values are likewise still pending from the clinic.
2. Google/Microsoft authorization model and tenant permissions — the outbox mechanism is already built and running on `internal/calendar.SandboxAdapter`, a fake that makes no outbound requests (see "Outbox/Calendar Delivery"); the real OAuth integration cannot begin until this item is approved. Credential storage has been provisionally decided as reusing `professional_calendars.calendar_ref_ciphertext` (application-layer encrypted into PostgreSQL); that table has not yet been created and will be added once a real provider is wired up.
3. Outbox retry interval, max attempts, and backoff — currently represented by the required environment variables `CALENDAR_OUTBOX_MAX_ATTEMPTS`, `CALENDAR_OUTBOX_RETRY_BACKOFF_SECONDS`, and `CALENDAR_WORKER_POLL_INTERVAL_SECONDS`, with `cmd/api`/`cmd/calendar-worker` failing to start if any is missing; actual values are still pending from the clinic. Who receives `dead_letter` alerts and the manual-handling SLA are still undefined — `internal/service/calendar.Service.Reconcile` currently only reports the backlog and does not integrate with any external notification system.
4. Session expiry (the actual `BOOKING_SESSION_TTL_MINUTES` value), the availability query range, and rate-limit values; the 24-hour expiry-cleanup job for `appointment_idempotency_keys` is not yet implemented — this is a known limitation.

## Open Items Before Backend Implementation

1. ~~The full production schema for BookingSession, Appointment, outbox, idempotency, and audit.~~ Resolved: the schema for BookingSession, Appointment, idempotency, audit, and `appointment_outbox` is defined under "Scheduling & Booking Production Data Model." The `professional_calendars` schema is defined but **not yet created as a migration** — it is only needed once a real provider's credentials are wired up.
2. ~~Each API endpoint's request/response schema, required fields, and complete status/error mapping.~~ Resolved: see "Scheduling/Booking Endpoint Schemas."
3. ~~The complete mapping from every combination of provider outbox states to `calendarDelivery`.~~ Resolved: see the mapping rules in "Outbox/Calendar Delivery" and `internal/service/calendar.Service.DeliveryStatus`; the `POST /appointments` response now includes a `calendarDelivery` field.
