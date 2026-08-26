# 後端架構與 Contract

## 文件狀態

本文件是待核准規格，尚未實作 Go 程式碼、migration、table 或 seeder。「必須」表示可驗證規則；未決定的產品設定集中列於「待診所確認」，不得在 production 使用隱性預設值。

## 架構決策

後端採 modular Go monolith 與 PostgreSQL。PostgreSQL 是預約、人員、服務、同步任務與 audit state 的唯一事實來源；Google Calendar 與 Microsoft Outlook 只是外部 projection。

AI 只能擷取 intent 與欄位候選值。服務資格、時長、狀態轉換、availability 與最終預約判定必須由確定性 domain code 完成。

### DDD 目錄規劃

```text
backend/
├── cmd/
│   ├── api/                    # HTTP API composition root
│   ├── calendar-worker/        # outbox delivery worker
│   └── migrate/                # migration/seed command
├── internal/
│   ├── catalog/                # Service and Professional
│   │   ├── domain/
│   │   ├── application/        # use cases and owned ports
│   │   └── adapters/
│   ├── scheduling/             # availability and interval rules
│   │   ├── domain/
│   │   ├── application/
│   │   └── adapters/
│   ├── booking/                # BookingSession and Appointment
│   │   ├── domain/
│   │   ├── application/
│   │   └── adapters/
│   ├── conversation/           # English-only flow and AI boundary
│   ├── calendar/               # delivery and reconciliation
│   ├── platform/               # HTTP, PostgreSQL, config, telemetry
│   └── shared/                 # approved shared value types only
├── migrations/
├── seeds/
├── test/
│   ├── contract/
│   └── integration/
└── README.md
```

`domain` 不得 import HTTP、SQL、AI SDK 或 Calendar SDK。介面由使用它的 `application` package 擁有，`adapters` 依賴並實作該介面。Context 間只傳遞 command、query 或已定義 value object，不共用 ORM model。`shared` 的內容必須至少被兩個 context 共用，且語意完全一致。

| Context | 責任 | 不負責 |
|---|---|---|
| Catalog | 服務、人員與明確服務資格 | Availability 與 Calendar 呼叫 |
| Scheduling | 營業規則、內部時段、外部 busy interval 與 availability | 建立 appointment |
| Booking | Session、最終確認、防重疊與 outbox transaction | 直接呼叫 Calendar SDK |
| Conversation | 英文對話、範圍邊界與 AI 候選值 | 核准預約 |
| Calendar | Outbox delivery、retry 與 reconciliation | 改變 appointment domain state |

Availability 是 Scheduling domain service；Calendar event 是 Appointment 的外部 projection。

## Canonical Types

### ID

- Aggregate/entity ID 與指向 entity 的 foreign key 一律使用 UUID v4。純 join table 可使用由 UUID foreign keys 組成的 composite primary key；`seed_history` 與 idempotency 等技術 table 使用本文明確定義的 natural key。
- UUID 由 application 透過可注入、使用 CSPRNG 的 `IDGenerator` 產生；禁止 sequential integer、語意 slug 與 nil UUID。
- PostgreSQL 使用 `uuid`；API 使用 RFC 9562 lowercase hyphenated UUID string。ID 建立後不得變更、重用或轉移。
- 不另建 `public_id`。`code` 是 immutable business key，不可代替 ID，且必須符合 `^[A-Z][A-Z0-9_]{0,31}$`。

### 時間與區間

- Production 必須設定有效 IANA clinic timezone；缺少或無效時 API 不得啟動。
- Instant 儲存為 PostgreSQL `timestamptz`，database connection timezone 固定 UTC，精度上限為 microseconds。
- API instant 使用 RFC 3339。Availability/appointment 時間必須含當時診所 UTC offset，並同時回傳 IANA `timeZone`。
- API `date` 使用 `YYYY-MM-DD` 並以 clinic timezone 解讀。時段一律使用 `[start, end)` 半開區間。
- `created_at` 與 `updated_at` 由 PostgreSQL 產生；`updated_at` 只在資料實際變更時更新。

### Version 與 Idempotency

- Aggregate `version` 使用 positive `bigint`，初始值 `1`，每次持久化的 domain state 變更原子遞增 `1`。
- API 以強 ETag 傳遞 version，格式為 `ETag: "3"`。修改 session 與建立 appointment 必須提供 `If-Match`；缺少時回傳 `428 PRECONDITION_REQUIRED`，不符時回傳 `412 SESSION_VERSION_MISMATCH`。Body 不重複傳送 version。
- `POST /appointments` 必須提供 `Idempotency-Key`：16–128 個 ASCII 字元，只允許英數字、`.`、`_`、`:` 與 `-`。
- Idempotency scope 為 method + route + public/authenticated session，retention 為 24 小時。同 key 與同 canonical request hash 回傳原 status/body；同 key 但不同 hash 回傳 `409 IDEMPOTENCY_KEY_REUSED`。併發同 key 只能有一筆 appointment。

## PostgreSQL-first 預約一致性

所有寫入一律先修改 PostgreSQL，成功 commit 後才能同步至外部系統。API request transaction 內不得呼叫 Google Calendar 或 Microsoft Graph 寫入 API。

1. 驗證 session、`If-Match`、`Idempotency-Key` 與患者輸入。
2. 讀取 PostgreSQL 內部時段，並重新讀取 Google 與 Microsoft busy intervals。任一必要 provider 無法驗證時回傳 `503 CALENDAR_AVAILABILITY_UNAVAILABLE`，不寫入 appointment。
3. 在單一 PostgreSQL transaction 再驗證 domain rules，建立 appointment、idempotency record、audit record 與 Google/Microsoft 各一筆 outbox record。
4. Commit 成功後 appointment 立即為已確認的內部事實，API 回傳 `201 Created` 與 `calendarDelivery=queued`。
5. Worker 先在 PostgreSQL transaction 鎖定 outbox row、將狀態改為 `processing` 並 commit，再使用 appointment ID + provider 派生的穩定 idempotency key 寫入外部 Calendar。外部呼叫完成後，必須將 delivery result 與 event reference 寫回 PostgreSQL。
6. 外部寫入失敗不得 rollback 或刪除 appointment；必須 retry，窮盡後進入 `dead_letter` 並告警人工處理。

### 防止重疊與狀態

- Appointment 必須儲存 `professional_id`、`start_at`、`end_at` 與 status，且 `start_at < end_at`。
- PostgreSQL 必須使用 exclusion constraint，禁止同一 `professional_id` 的 `confirmed` appointment 重疊 `tstzrange(start_at, end_at, '[)')`。Constraint conflict 映射為 `409 SLOT_NO_LONGER_AVAILABLE`。
- BookingSession：`collecting`、`readyToConfirm`、`confirmed`、`expired`。只允許 `collecting → readyToConfirm`、`readyToConfirm → collecting|confirmed` 與任一未終止狀態 `→ expired`；`confirmed` 與 `expired` 是 terminal state。
- Appointment：`confirmed`、`cancelled`；第一版不提供患者端 cancellation transition。
- Provider outbox：`pending`、`processing`、`retryable`、`delivered`、`dead_letter`。
- API `calendarDelivery`：`queued`、`partial`、`delivered`、`attentionRequired`。兩方都成功才是 `delivered`；一方成功為 `partial`；任一方 `dead_letter` 為 `attentionRequired`。

## Catalog Production Data Model

欄位除非標示 nullable，否則必須 `NOT NULL`。Constraint 必須在 PostgreSQL 建立，不可只依賴 Go validation。

### `services`

| Column | Type | Constraint |
|---|---|---|
| `id` | `uuid` | Primary key |
| `code` | `varchar(32)` | Unique immutable business key |
| `display_name` | `varchar(100)` | Trim 後 1–100 Unicode code points |
| `duration_minutes` | `smallint` | `> 0` |
| `is_active` | `boolean` | Default `true` |
| `created_at`, `updated_at` | `timestamptz` | Default database current time |

### `professionals`

| Column | Type | Constraint |
|---|---|---|
| `id` | `uuid` | Primary key；API 使用此 ID |
| `code` | `varchar(32)` | Unique immutable business key |
| `display_name` | `varchar(100)` | Trim 後 1–100 Unicode code points；核准的英文名稱 |
| `is_active` | `boolean` | Default `true`；`false` 時不產生新 availability |
| `created_at`, `updated_at` | `timestamptz` | Default database current time |

`Professional` 不儲存 `level`。服務資格只來自明確關聯，不得從 code 或 display name 推導。Professional 不得 hard delete；停用不得改變既有 appointment。

### `professional_service_qualifications`

| Column | Type | Constraint |
|---|---|---|
| `professional_id` | `uuid` | FK → `professionals.id ON DELETE RESTRICT` |
| `service_id` | `uuid` | FK → `services.id ON DELETE RESTRICT` |
| `created_at` | `timestamptz` | Default database current time |

Primary key 為 (`professional_id`, `service_id`)。不儲存 duration 或 service-code array。移除資格前必須確認沒有相關未來 appointment。

### `professional_calendars`

| Column | Type | Constraint |
|---|---|---|
| `id` | `uuid` | Primary key |
| `professional_id` | `uuid` | FK → `professionals.id ON DELETE RESTRICT` |
| `provider` | `varchar(16)` | Check: `google` or `microsoft` |
| `calendar_ref_ciphertext` | `bytea` | Application-layer encrypted reference |
| `encryption_key_id` | `varchar(128)` | Managed-key identifier；不是 key material |
| `is_active` | `boolean` | Default `true` |
| `verified_at` | `timestamptz` | Nullable |
| `created_at`, `updated_at` | `timestamptz` | Default database current time |

(`professional_id`, `provider`) 必須 unique。OAuth token、client secret 與 private key 不得存於 table、seed 或 repository，必須由受管理 secret system 持有。Provider 未設定、未驗證或不可用時 fail-closed。

## Reference-data Seeder

Seeder 是受權限的明確維運指令，不得在 API startup 自動執行。

| Professional code | Display name | Qualifications |
|---|---|---|
| `JUNIOR` | `Junior` | A、B |
| `SENIOR_1` | `Senior 1` | A、B、C、D、E |
| `SENIOR_2` | `Senior 2` | A、B、C、D、E |

Seeder 先驗證 A=60、B=60、C=150、D=120、E=360 分鐘。任一服務不存在、重複或 duration 不符時，整個 transaction rollback。

1. Seed artifact 具有 immutable version 與 SHA-256 checksum；成功紀錄寫入 `seed_history(version, checksum, executed_at, executor_id)`，`version` 為 primary key。相同 version/checksum 已存在時成功 no-op；相同 version 但 checksum 不同時 fail。
2. 以 `code` 查找人員。不存在時產生 UUID 並 insert；已存在時只驗證靜態欄位，不覆寫、不重新啟用，差異時 fail。
3. 只 insert 缺少的 12 筆 qualification；不自動刪除額外人員或資格。資料變更使用新版本 artifact。
4. 驗證、insert 與 `seed_history` 寫入在單一 PostgreSQL transaction 完成。
5. Calendar mapping、email、demo ID 與 credential 不得出現於 seed。

驗收涵蓋首次/重複執行、checksum/靜態欄位衝突、rollback、FK/unique constraint、12 筆資格組合與停用人員。

## RESTful API Contract

Base path 為 `/api/v1`，只接受 HTTPS UTF-8 JSON。Request body 上限 64 KiB；name 為 trim 後 1–100 Unicode code points，email 最長 254 ASCII characters，message 最長 2,000 Unicode code points。

| Method | Path | 用途 |
|---|---|---|
| `GET` | `/services` | 取得啟用服務 |
| `GET` | `/professionals?serviceCode=C` | 取得合格且啟用的人員 |
| `GET` | `/availability?serviceCode=C&date=2026-09-02` | 取得匿名時段 |
| `POST` | `/booking-sessions` | 建立短期 session |
| `GET` | `/booking-sessions/{id}` | 取得 session |
| `PATCH` | `/booking-sessions/{id}` | 使用 `If-Match` 更新 session |
| `POST` | `/booking-sessions/{id}/messages` | 傳送英文患者訊息 |
| `POST` | `/appointments` | 使用 `If-Match` 與 `Idempotency-Key` 確認預約 |

Appointment body 只含 UUID `bookingSessionId`，不包含 version。Booking session 的 `selectedSlot` 必須含 UUID `professionalId`、RFC 3339 `start`/`end` 與 IANA `timeZone`。

Public session 使用 `Secure; HttpOnly; SameSite=Strict` cookie、CSRF token 與 non-empty exact-origin allowlist。Rate-limit 數值由環境設定，未設定時 production 不得啟動；超限回傳 `429` 與 `Retry-After`。第一版不提供 appointment list、取消或改期 API。

### Error Contract

錯誤使用 `application/problem+json`，必須含 `type`、`title`、`status`、`code`、`detail` 與 request-scoped `instance`；欄位錯誤可含 `errors[]` 的 `field` 與 `code`。`detail` 必須是安全的簡明英文，不得包含 stack trace、SQL、患者資料、token 或 provider response。

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
| `503` | `CALENDAR_AVAILABILITY_UNAVAILABLE` |

## Calendar Adapter Contract

Provider-neutral ports 至少支援 `Busy`、`Create`、`Update`、`Cancel`、health、retry classification 與 reconciliation。`Cancel` 只能在未來核准 cancellation domain flow 後使用。

Google 與 Microsoft 授權模式必須分別在 sandbox 驗證後核准，不預設共用同一 OAuth flow。各 provider 必須使用 least-privilege access，並記錄 credential owner、rotation、revocation、reauthorization 與 tenant/calendar access boundary。

## 安全與維運底線

- Production 必須有 TLS、受管理 secrets、加密 storage/backup、point-in-time recovery、員工 SSO/RBAC 與 audit trail。
- Log、trace、metric 與 alert 不得包含患者訊息、姓名、email、token、Calendar reference 明文或 provider response body。Audit 只使用 entity ID、action、actor ID 與 timestamp。
- 處理患者資料前必須完成適用的隱私/健康資料審查與 vendor agreement。
- 每日監控 outbox backlog、`dead_letter`、provider health 與預約衝突；每季進行 access review、backup restore 與 provider outage drill。

規劃基準為 8–12 週、USD 90k–165k。每月 3,000 次對話與 600 筆預約時，純基礎設施約 USD 100–600/月。此為採購前必須重新確認的規劃數字，不是固定報價。

## 待診所確認

1. Clinic IANA timezone、營業時間、假日、休息時段、slot interval 與最短提前預約時間。
2. A–E 的最終英文顯示名稱。
3. Google/Microsoft 授權模式、tenant 權限與 credential storage。
4. Outbox retry interval、最大次數、`dead_letter` 告警對象與人工處理 SLA。
5. Session 過期時間、availability 查詢範圍、rate-limit 數值與資料 retention/deletion 週期。
