# 後端架構與 Contract

## 文件狀態

本文件是已核准的後端基準 contract。「必須」表示可驗證規則；未決定的產品設定集中列於「待診所確認」，不得在 production 使用隱性預設值。尚缺的實作級 contract 集中列於「後端實作前待釐清」。

## 架構決策

後端規劃採 modular Go monolith 與 PostgreSQL，分層採 handler / service / repository，不採 DDD bounded context 分層。PostgreSQL 是預約、人員、服務、availability、同步任務與 audit state 的唯一事實來源；Google Calendar 與 Microsoft Outlook 是只接收 PostgreSQL 資料的外部 projection，不得反向影響 availability 或預約判定。

### 技術棧

| 用途 | 套件 | 使用範圍 |
|---|---|---|
| HTTP router/middleware | [`gin-gonic/gin`](https://github.com/gin-gonic/gin) | 只用於 `internal/handler` 與 `internal/platform` 的 HTTP server 組裝；`service`、`repository` 不得 import Gin。 |
| PostgreSQL ORM | [`gorm.io/gorm`](https://gorm.io/) | 唯一的 `*gorm.DB` connection pool 由 `internal/platform/database` 建立並透過 Fx 以單例注入；查詢邏輯只能寫在 `internal/repository`，`service` 與 `handler` 不得 import GORM 或操作 `*gorm.DB`。禁止使用 `AutoMigrate`，schema 變更一律走既有 migration 流程。 |
| Dependency injection | [`go.uber.org/fx`](https://github.com/uber-go/fx) | 只用於 `cmd/api`、`cmd/calendar-worker` 的 composition root，負責組裝 handler/service/repository 與 platform 依賴；business package 不得 import Fx。 |
| 環境變數載入 | [`joho/godotenv`](https://github.com/joho/godotenv) | 只在本機/開發環境載入 `.env`；production 必須以真正的環境變數提供設定，缺少必要變數時 API 不得啟動，不得以 `.env` 作為 production 設定來源。 |

AI 只能擷取 intent 與欄位候選值。服務資格、時長、狀態轉換、availability 與最終預約判定必須由確定性 service 層程式碼完成。

### 目錄規劃

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

`handler` 只負責 HTTP request/response 轉換、驗證輸入格式與呼叫 `service`，不得直接操作 SQL 或呼叫外部 SDK。`service` 持有商業邏輯與其所需外部依賴的 interface（如 repository、calendar adapter），不得 import HTTP 或 SQL driver。`repository` 只實作 `service` 定義的 interface 並操作 PostgreSQL，不得包含商業規則。功能模組間以 exported service method、request/response 或 value object 溝通，不共用 repository 或 ORM model；`shared` 只放置跨兩個以上模組且語意一致的型別。

| 模組 | 責任 | 不負責 |
|---|---|---|
| Catalog | 服務、人員與明確服務資格 | Availability 與 Calendar 呼叫 |
| Scheduling | 營業規則、內部時段、PostgreSQL appointment 與 availability | 建立 appointment 或讀取外部 Calendar |
| Booking | Session、最終確認、防重疊與 outbox transaction | 直接呼叫 Calendar SDK |
| Conversation | 英文對話、範圍邊界與 AI 候選值 | 核准預約 |
| Calendar | Outbox delivery、retry 與 reconciliation | 改變 appointment 狀態 |

Availability 是 Scheduling service 的職責；Calendar event 是 Appointment 的外部 projection。

## Canonical Types

### ID

- Aggregate/entity ID 與指向 entity 的 foreign key 一律使用 UUID v4。純 join table 可使用由 UUID foreign keys 組成的 composite primary key；`seed_history` 與 idempotency 等技術 table 使用本文明確定義的 natural key。
- UUID 由 application 透過可注入、使用 CSPRNG 的 `IDGenerator` 產生；禁止 sequential integer、語意 slug 與 nil UUID。
- PostgreSQL 使用 `uuid`；API 使用 RFC 9562 lowercase hyphenated string。ID 建立後不得變更、重用或轉移。
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
- Idempotency scope 為 method + route + JWT `sub`，retention 為 24 小時。同 key 與同 canonical request hash 回傳原 status/body；同 key 但不同 hash 回傳 `409 IDEMPOTENCY_KEY_REUSED`。併發同 key 只能有一筆 appointment。

## PostgreSQL-first 預約一致性

API request transaction 只寫 PostgreSQL；外部 Calendar 寫入必須在 commit 後執行。

1. 驗證 session、`If-Match`、`Idempotency-Key` 與患者輸入。
2. 只從 PostgreSQL 讀取營業規則、內部保留時段與既有 appointment；不得查詢 Google 或 Microsoft busy intervals。
3. 在單一 PostgreSQL transaction 重新驗證 availability 與 domain rules，建立 appointment、idempotency record、audit record，以及 Google、Microsoft 各一筆 outbox record。
4. Commit 成功後 appointment 立即為已確認的內部事實，API 回傳 `201 Created` 與 `calendarDelivery=queued`。
5. Worker 在 PostgreSQL transaction 鎖定 outbox row、標記為 `processing` 並 commit，再以 appointment ID 與 provider 派生的穩定 idempotency key 寫入外部 Calendar，最後回寫 delivery result 與 event reference。
6. 外部寫入失敗不得 rollback 或刪除 appointment；必須 retry，窮盡後進入 `dead_letter` 並告警人工處理。

### 防止重疊與狀態

- Appointment 必須儲存 `professional_id`、`start_at`、`end_at` 與 status，且 `start_at < end_at`。
- PostgreSQL 必須使用 exclusion constraint，禁止同一 `professional_id` 的 `confirmed` appointment 重疊 `tstzrange(start_at, end_at, '[)')`。Constraint conflict 映射為 `409 SLOT_NO_LONGER_AVAILABLE`。
- BookingSession：`collecting`、`readyToConfirm`、`confirmed`、`expired`。只允許 `collecting → readyToConfirm`、`readyToConfirm → collecting|confirmed` 與任一未終止狀態 `→ expired`；`confirmed` 與 `expired` 是 terminal state。
- Appointment：第一版只有 `confirmed`，不定義 cancellation transition。
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

(`professional_id`, `provider`) 必須 unique。開始接受預約前，每位啟用的 professional 必須同時具有一筆啟用且已驗證的 Google mapping，以及一筆啟用且已驗證的 Microsoft mapping。OAuth token、client secret 與 private key 不得存於 table、seed 或 repository，必須由受管理 secret system 持有。Mapping 或 provider 暫時不可用不影響 PostgreSQL 預約判定；同步由 outbox retry 與 reconciliation 處理。

### `seed_history`

| Column | Type | Constraint |
|---|---|---|
| `version` | `varchar(32)` | Primary key（natural key，非 UUID） |
| `checksum` | `varchar(64)` | SHA-256 hex digest，NOT NULL |
| `executed_at` | `timestamptz` | Default database current time |
| `executor_id` | `varchar(128)` | NOT NULL，記錄執行者身分 |

`checksum` 使用 `varchar` 而非 `char`：`char(n)` 會將值以空白填滿至固定長度，讀回時比對會因尾端空白而失準。無 FK 關聯——`seed_history` 是純技術/稽核表。

## Scheduling & Booking Production Data Model

欄位除非標示 nullable，否則必須 `NOT NULL`。Constraint 必須在 PostgreSQL 建立，不可只依賴 Go validation。診所營業時間、假日與內部保留時段屬於「可預約時間」的判斷輸入，依根目錄 AGENTS.md 非協商規則必須存於 PostgreSQL，不得以環境變數或程式碼常數表示；slot interval 與最短提前預約時間屬於運算參數，走 `internal/platform/config` 的必填環境變數（無預設值，缺少時 API 不得啟動），與 `CLINIC_TIMEZONE` 相同模式。

### `clinic_hours`

| Column | Type | Constraint |
|---|---|---|
| `day_of_week` | `smallint` | Primary key，`0`（Sunday）–`6`（Saturday） |
| `is_open` | `boolean` | NOT NULL |
| `open_time` | `time` | Nullable；`is_open = false` 時必須為 `NULL` |
| `close_time` | `time` | Nullable；`is_open = false` 時必須為 `NULL`，`is_open = true` 時必須 `> open_time` |

固定 7 筆（每個 weekday 一筆）。資料表可為空——診所尚未確認實際時數前，空表代表「無法判斷可預約時間」，Availability 依 fail-closed 規則回傳空結果，不得臆測預設營業時間。

### `clinic_closures`

| Column | Type | Constraint |
|---|---|---|
| `closure_date` | `date` | Primary key |
| `reason` | `varchar(200)` | Nullable |

單日全休（例如國定假日）。跨日休診以多筆 row 表示，不支援區間欄位。

### `professional_blocked_slots`

| Column | Type | Constraint |
|---|---|---|
| `id` | `uuid` | Primary key |
| `professional_id` | `uuid` | FK → `professionals.id ON DELETE RESTRICT` |
| `start_at`, `end_at` | `timestamptz` | NOT NULL，`start_at < end_at` |
| `reason` | `varchar(200)` | Nullable |
| `created_at` | `timestamptz` | Default database current time |

代表「內部保留時段」（例如個人休假、行政時段）。第一版沒有對應 API，由維運人員直接寫入 PostgreSQL；Availability 計算時必須排除與此表重疊的候選時段。

### `booking_sessions`

| Column | Type | Constraint |
|---|---|---|
| `id` | `uuid` | Primary key |
| `status` | `varchar(16)` | Check：`collecting`、`readyToConfirm`、`confirmed`、`expired` |
| `service_id` | `uuid` | Nullable，FK → `services.id ON DELETE RESTRICT` |
| `professional_id` | `uuid` | Nullable，FK → `professionals.id ON DELETE RESTRICT` |
| `slot_start_at`, `slot_end_at` | `timestamptz` | Nullable，成對出現，`slot_start_at < slot_end_at` |
| `slot_time_zone` | `varchar(64)` | Nullable，IANA 名稱 |
| `patient_name` | `varchar(100)` | Nullable，trim 後 1–100 Unicode code points |
| `patient_email` | `varchar(254)` | Nullable，最長 254 ASCII characters |
| `version` | `bigint` | NOT NULL，預設 `1`，依 Canonical Types「Version 與 Idempotency」原子遞增 |
| `expires_at` | `timestamptz` | NOT NULL |
| `created_at`, `updated_at` | `timestamptz` | Default database current time |

狀態轉換依「防止重疊與狀態」既有規則：只允許 `collecting → readyToConfirm`、`readyToConfirm → collecting|confirmed`，以及任一未終止狀態 `→ expired`；`confirmed` 與 `expired` 為 terminal state。`expires_at` 於建立時以 `BOOKING_SESSION_TTL_MINUTES` 設定計算；任何讀取/更新操作前都必須先檢查 `expires_at < now()`，逾期一律視為 `expired`（`410 BOOKING_SESSION_EXPIRED`），不因為尚未有背景 job 把 `status` 欄位改掉而放行。

### `appointments`

| Column | Type | Constraint |
|---|---|---|
| `id` | `uuid` | Primary key |
| `booking_session_id` | `uuid` | Unique，FK → `booking_sessions.id ON DELETE RESTRICT` |
| `service_id` | `uuid` | FK → `services.id ON DELETE RESTRICT` |
| `professional_id` | `uuid` | FK → `professionals.id ON DELETE RESTRICT` |
| `patient_name` | `varchar(100)` | NOT NULL |
| `patient_email` | `varchar(254)` | NOT NULL |
| `start_at`, `end_at` | `timestamptz` | NOT NULL，`start_at < end_at` |
| `time_zone` | `varchar(64)` | NOT NULL，IANA 名稱 |
| `status` | `varchar(16)` | Check：第一版僅 `confirmed`（無 cancellation transition） |
| `created_at`, `updated_at` | `timestamptz` | Default database current time |

必須建立 `EXCLUDE USING gist (professional_id WITH =, tstzrange(start_at, end_at, '[)') WITH &&) WHERE (status = 'confirmed')`（需 `CREATE EXTENSION IF NOT EXISTS btree_gist`），實作既有「防止重疊與狀態」規則；constraint conflict 映射為 `409 SLOT_NO_LONGER_AVAILABLE`。

### `appointment_idempotency_keys`

| Column | Type | Constraint |
|---|---|---|
| `key` | `varchar(128)` | Primary key（natural key，非 UUID），16–128 個 ASCII 字元，只允許英數字、`.`、`_`、`:`、`-` |
| `request_hash` | `varchar(64)` | NOT NULL，canonical request 的 SHA-256 hex digest |
| `appointment_id` | `uuid` | Nullable，FK → `appointments.id ON DELETE RESTRICT` |
| `response_status` | `smallint` | NOT NULL |
| `response_body` | `jsonb` | NOT NULL |
| `created_at` | `timestamptz` | Default database current time |

Scope 為 method + route + JWT `sub`（已於 Canonical Types 定義），此表只補 schema。同 `key` 與同 `request_hash` 時回放既有 `response_status`/`response_body`；同 `key` 不同 `request_hash` 回傳 `409 IDEMPOTENCY_KEY_REUSED`。Retention 24 小時；本階段未實作過期清除 job，屬已知限制，見「待診所確認」。

### `appointment_audit_log`

| Column | Type | Constraint |
|---|---|---|
| `id` | `uuid` | Primary key |
| `entity_id` | `uuid` | NOT NULL |
| `action` | `varchar(64)` | NOT NULL |
| `actor_id` | `varchar(128)` | NOT NULL |
| `created_at` | `timestamptz` | Default database current time |

只存 entity ID、action、actor ID 與 timestamp，符合「安全與維運底線」對 log／audit 的限制；不得寫入患者姓名、email 或訊息內容。

### `appointment_outbox`

| Column | Type | Constraint |
|---|---|---|
| `id` | `uuid` | Primary key |
| `appointment_id` | `uuid` | FK → `appointments.id ON DELETE RESTRICT` |
| `provider` | `varchar(16)` | Check: `google` or `microsoft` |
| `status` | `varchar(16)` | Check: `pending`、`processing`、`retryable`、`delivered`、`dead_letter` |
| `idempotency_key` | `varchar(128)` | Unique；`appt:{appointmentId}:{provider}`，見「PostgreSQL-first 預約一致性」步驟 5 |
| `attempt_count` | `integer` | Default `0` |
| `next_attempt_at` | `timestamptz` | Default database current time |
| `event_reference` | `varchar(512)` | Nullable，adapter 回傳的 provider event reference |
| `last_error` | `varchar(500)` | Nullable，經 sanitize 的失敗原因，不得含 provider response body（見「安全與維運底線」） |
| `created_at`、`updated_at` | `timestamptz` | Default database current time |

(`appointment_id`, `provider`) 必須 unique——每筆 appointment 每個 provider 恰有一筆 outbox row。`internal/service/booking` 在確認 appointment 的同一 transaction 內建立 google、microsoft 各一筆 `pending` row；`internal/service/calendar`／`cmd/calendar-worker` 依「PostgreSQL-first 預約一致性」步驟 5–6 推進其餘狀態，不改變 appointment 狀態。

### Outbox／Calendar delivery（已實作：outbox 機制 + sandbox adapter；真實 Google/Microsoft OAuth 尚未串接）

`POST /appointments` 成功後，在同一 transaction 內為 google、microsoft 各建立一筆 `appointment_outbox` pending row，並於回應中加入 `calendarDelivery=queued`（見下方 endpoint schema）。`cmd/calendar-worker` 輪詢到期 row，鎖定並標記 `processing` 後 commit，再呼叫 `internal/service/calendar.Adapter.Create`；成功寫回 `delivered` 與 `event_reference`，暫時性失敗依 `CALENDAR_OUTBOX_RETRY_BACKOFF_SECONDS`（指數退避）標記 `retryable` 並排入 `next_attempt_at`，達到 `CALENDAR_OUTBOX_MAX_ATTEMPTS` 或永久性失敗則標記 `dead_letter`。`calendarDelivery` 由 `internal/service/calendar.Service.DeliveryStatus` 即時查詢兩筆 outbox row 計算，不快取：兩者皆 `delivered` 才是 `delivered`；一方 `delivered` 為 `partial`；任一方 `dead_letter` 為 `attentionRequired`；其餘為 `queued`。外部寫入失敗不回滾或刪除已 commit 的 appointment，符合「PostgreSQL-first 預約一致性」。

目前唯一的 `Adapter` 實作是 `internal/calendar.SandboxAdapter`：一個不對外發出任何網路請求的 deterministic fake，回傳 `sandbox:{provider}:{idempotencyKey}` 作為 event reference。這是為了在真實 OAuth 授權模式核准前，先把 outbox schema、worker、retry/dead_letter 狀態機與 `calendarDelivery` 映射建置並測試完成；**它不是 Google 或 Microsoft Calendar 的真實整合，不得對外宣稱為 production-ready Calendar sync**。`professional_calendars` table（見上方 schema）尚未建立——它只在接上真實 provider 憑證時才需要，目前的 sandbox worker 不查詢也不依賴它。真實整合待「Google/Microsoft 授權模式、tenant 權限與 credential storage」（見「待診所確認」第 2 項）核准後才能開始：屆時的 credential 儲存方式已初步決定為 application-layer 加密後存入 `professional_calendars.calendar_ref_ciphertext`（沿用既有欄位設計），但實際 OAuth flow、tenant 權限與該 table 的建立仍待 sandbox 驗證與診所核准。

Reconciliation 目前僅提供 `internal/service/calendar.Service.Reconcile`，回傳目前的 `dead_letter` backlog 供呼叫端記錄／告警；實際告警對象與人工處理 SLA 仍待診所確認（見「待診所確認」第 3 項），因此這裡刻意不接任何外部通知系統。

## Reference-data Seeder

Seeder 是受權限的明確維運指令，不得在 API startup 自動執行。下表數值必須與根目錄 README「診所模型」一致，此處列出是為了提供 seed artifact 的精確 insert 值。

| Service code | Display name | Duration |
|---|---|---:|
| `A` | `Service A` | 60 minutes |
| `B` | `Service B` | 60 minutes |
| `C` | `Service C` | 150 minutes |
| `D` | `Service D` | 120 minutes |
| `E` | `Service E` | 360 minutes |

| Professional code | Display name | Qualifications |
|---|---|---|
| `JUNIOR` | `Junior` | A、B |
| `SENIOR_1` | `Senior 1` | A、B、C、D、E |
| `SENIOR_2` | `Senior 2` | A、B、C、D、E |

1. Seed artifact 具有 immutable version 與 SHA-256 checksum；成功紀錄寫入 `seed_history(version, checksum, executed_at, executor_id)`，`version` 為 primary key。相同 version/checksum 已存在時成功 no-op；相同 version 但 checksum 不同時 fail。
2. 以 `code` 查找服務與人員。不存在時產生 UUID 並 insert；已存在時驗證上述固定 display name、duration 與其他靜態欄位，不覆寫、不重新啟用，差異時 fail。
3. 只 insert 缺少的 12 筆 qualification；不自動刪除額外人員或資格。固定資料變更必須使用新版本 artifact。
4. 所有驗證、insert 與 `seed_history` 寫入在單一 PostgreSQL transaction 完成；任一衝突都必須 rollback。
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
| `POST` | `/voice/transcriptions` | 上傳錄音，回傳 AI 轉錄文字 |
| `POST` | `/appointments` | 使用 `If-Match` 與 `Idempotency-Key` 確認預約 |

Appointment body 只含 UUID `bookingSessionId`，不包含 version。Booking session 的 `selectedSlot` 必須含 UUID `professionalId`、RFC 3339 `start`/`end` 與 IANA `timeZone`。

患者身分不透過帳號或登入驗證：booking session 於 `collecting` 狀態蒐集患者姓名與 email，僅作為該筆預約的聯絡方式與行事曆邀請對象。任何外部 email 皆可用於預約，不要求預先註冊、網域限制或所有權驗證；email 不作為長期帳號識別，也不得用於跨 session 查詢或關聯患者歷史預約。

Public session 必須使用簽署 JWT，並透過 `Secure; HttpOnly; SameSite=Strict` cookie 傳送，不得存入 `localStorage` 或 `sessionStorage`。JWT 至少包含 `iss`、`aud`、`sub`、`jti`、`iat`、`nbf` 與 `exp`，不得包含患者姓名、email 或訊息；伺服器必須固定允許的簽章演算法並驗證所有 claims。Cookie authentication 仍須搭配 CSRF token 與 non-empty exact-origin allowlist。

Rate-limit 數值由環境設定，未設定時 production 不得啟動；超限回傳 `429` 與 `Retry-After`。第一版不提供 appointment list、取消或改期 API。

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
| `503` | `AVAILABILITY_UNAVAILABLE` / `VOICE_TRANSCRIPTION_UNAVAILABLE` |
| `500` | `INTERNAL_ERROR` |

### Catalog Endpoint Schemas

`GET /services` 回傳啟用中的服務陣列：

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

`GET /professionals?serviceCode=C` 回傳同時啟用且具備該服務資格的人員陣列：

```json
[
  {
    "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
    "code": "SENIOR_1",
    "displayName": "Senior 1"
  }
]
```

`serviceCode` 為必填 query 參數，必須符合 `^[A-Z][A-Z0-9_]{0,31}$`；缺少時回傳 `400 INVALID_REQUEST`（`errors[].field="serviceCode"`、`code="REQUIRED"`），格式不符回傳同狀態碼（`code="INVALID_FORMAT"`）。`serviceCode` 格式正確但不存在啟用中的對應服務、或該服務目前無任何啟用中的合格人員，回傳 `200` 與空陣列 `[]`——此為合法查詢的合法結果，不視為錯誤。

### Scheduling／Booking Endpoint Schemas

`GET /availability?serviceCode=C&date=2026-09-02` 回傳依 `serviceCode` 時長、合格人員、`clinic_hours`、`clinic_closures`、`professional_blocked_slots` 與既有 `confirmed` appointment 計算出的匿名候選時段：

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

`serviceCode` 規則同 Catalog；`date` 為必填 `YYYY-MM-DD`，以 `CLINIC_TIMEZONE` 解讀，格式不符回傳 `400 INVALID_REQUEST`（`code="INVALID_FORMAT"`）。以下情況回傳 `200` 與空陣列 `[]`（合法空結果，非錯誤）：`serviceCode` 無合格人員、當日 `clinic_hours.is_open=false`、`clinic_hours` 尚無該 weekday 資料、當日為 `clinic_closures`、或扣除既有預約與 blocked slot 後無剩餘可預約時段。無法連線 PostgreSQL 或查詢失敗時，依 fail-closed 規則回傳 `503 AVAILABILITY_UNAVAILABLE`，不得回傳臆測結果。

`POST /booking-sessions` body 可為空物件 `{}`；建立 `status="collecting"` 的新 session，回傳 `201`、`ETag: "1"`、`Location: /api/v1/booking-sessions/{id}`，body 為下方 session representation。

`GET /booking-sessions/{id}` 回傳目前 session：

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

`ETag` header 帶目前 `version`。`id` 不存在或已過期（`expires_at < now()`）回傳 `410 BOOKING_SESSION_EXPIRED`。

`PATCH /booking-sessions/{id}`（`If-Match` 必填，缺少回傳 `428 PRECONDITION_REQUIRED`，值與目前 `version` 不符回傳 `412 SESSION_VERSION_MISMATCH`）允許局部更新：

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

所有欄位皆為選填，只更新請求中出現的欄位。`status` 只接受顯式轉換目標值，非法轉換（見「防止重疊與狀態」狀態機）回傳 `422 VALIDATION_FAILED`。轉入 `readyToConfirm` 前，service 層必須確認 `serviceCode`、`selectedSlot`、`patientName`、`patientEmail` 皆已填妥，且 slot 仍在 `GET /availability` 會回傳的範圍內；否則回傳 `422 VALIDATION_FAILED` 並於 `errors[]` 標示缺漏欄位。成功回應 `200` 與更新後的 session representation，`ETag` 遞增為新 `version`。

`POST /appointments`（`If-Match` 為 session 目前 `version`；`Idempotency-Key` 必填，格式見 Canonical Types）body 只含 `bookingSessionId`：

```json
{ "bookingSessionId": "3fa85f64-5717-4562-b3fc-2c963f66afa6" }
```

成功時在單一 transaction 內重新驗證 slot 可用性、建立 `appointments` row、`appointment_idempotency_keys` row、`appointment_audit_log` row 與 google／microsoft 各一筆 `appointment_outbox` pending row，session 轉為 `confirmed`；回傳 `201`：

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

`calendarDelivery` 剛確認時一律為 `queued`；同一 `Idempotency-Key`／`request_hash` 的重放請求會即時查詢兩筆 outbox row 目前狀態，可能回傳 `queued`、`partial`、`delivered` 或 `attentionRequired`（見「Outbox／Calendar delivery」章節的映射規則），不是固定值。Session 非 `readyToConfirm` 狀態時回傳 `422 VALIDATION_FAILED`；slot 已被搶走（exclusion constraint 衝突）回傳 `409 SLOT_NO_LONGER_AVAILABLE`；`Idempotency-Key` 重複且 request hash 不同回傳 `409 IDEMPOTENCY_KEY_REUSED`；重複且 hash 相同回放原本的 status/body。

`POST /booking-sessions/{id}/messages`（不需要 `If-Match`——這個 endpoint 內部讀取 session 目前 version 並套用變更，不對外暴露樂觀鎖）傳送一則英文患者訊息：

```json
{ "message": "I'd like to book a cleaning next Tuesday afternoon" }
```

`message` 為必填，trim 後 1–2,000 Unicode code points（同本節開頭的全域訊息長度限制）；缺少或空白回傳 `400 INVALID_REQUEST`（`errors[].field="message"`、`code="REQUIRED"`），超過長度上限回傳同狀態碼（`code="TOO_LONG"`）。Session 不存在或已過期回傳 `410 BOOKING_SESSION_EXPIRED`，與其他 session endpoint 一致。

成功時回傳 `200`，body 與 `GET /booking-sessions/{id}` 相同的 session representation，外加三個欄位：

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

- `reply`：機器人回覆的英文文字。**一律由後端確定性樣板組成**——AI 只用來判斷本輪要用哪個樣板（例如「已擷取到 service，但缺 date」或「超出範圍分類為 emergency」），不得把 AI 生成的自由文字直接當作 `reply` 回傳，避免 AI 意外產出診斷、報價或其他超出範圍的內容。
- `offeredSlots`：本輪由 Scheduling（`GET /availability` 同一套邏輯）算出的候選時段陣列，形狀同 `GET /availability` 的元素（`professionalId`/`start`/`end`/`timeZone`）；只有當 session 已知 `serviceCode` 且本輪訊息可解析出明確日期時才非空，其餘情況一律回傳 `[]`。這份候選清單同時會存入 session 的 `offered_slots`（內部欄位，不對外回傳），作為**有限的「上一輪」記憶**——不是完整多輪對話歷史，只反查緊接在前一輪提供過的候選：若本輪訊息沒有再給新的 `serviceCode`／日期，且用序數（「the first one」）或時段（「the morning one」）指代其中之一，`/messages` 會依伺服器對候選清單依 `start` 由早到晚排序（同時間依 `professionalId` 排序求穩定）後的順位解析出對應候選，並在套用前重新呼叫一次 `GetAvailability` 現查確認仍可預約（fail-closed，不信任已儲存的候選）；若確認仍可預約才寫入 `selectedSlot` 並清空 `offered_slots`，若已被搶走則不套用、重新提供最新候選並清空舊的。序數只涵蓋 1–5（對應候選上限 `maxOfferedSlots`）與「最後一個」，不解析確切時間（如「9am」）；無法判斷或超出範圍時直接忽略，回到今天既有的正常流程，不視為錯誤。**已知限制**：若患者前端當下有套用執業者篩選，畫面上視覺的「第一個」可能與伺服器認定的順位不同，此情況不隱藏、寫在此處供實作與測試參考。除了這條指代路徑，選擇某個時段一律也能透過既有 `PATCH /booking-sessions/{id}`（帶 `selectedSlot`）完成，行為不變。
- `outOfScope`：本輪訊息是否被分類為超出範圍（診斷、處方、緊急醫療、報價、保險或取消／改期要求之一）。分類為 `true` 時，`reply` 一律是該分類對應的固定樣板（引導患者直接聯絡診所，緊急情況建議立即就醫），且本輪**不會**修改任何 session 欄位，即使訊息中同時夾帶了服務或日期資訊。

`ETag` header 一律帶 session 目前的 version（不論本輪是否真的修改了欄位）。AI 候選值若無法對應到合法的服務代碼或候選日期無法解析，**不是錯誤**——service 層直接忽略該候選值，`reply` 改為澄清問句請患者換句話說，HTTP 回應仍是 `200`。同理，若套用當下 session version 被其他請求搶先變更，service 內部重新讀取一次並重試一次即可，仍回傳 `200`（`reply` 提示請再說一次），不會把樂觀鎖衝突以 `412` 曝露給聊天使用者。真正會回傳非 `200` 的情況只有：請求格式本身不合法（`400`）、session 不存在或過期（`410`）、計算 `offeredSlots` 時 PostgreSQL 查詢失敗（依 fail-closed 規則回傳 `503 AVAILABILITY_UNAVAILABLE`，不得回傳臆測時段）、或非預期的伺服器錯誤（`500`）。

## Calendar Adapter Contract

Provider-neutral port（`internal/service/calendar.Adapter`）只負責將 PostgreSQL appointment 投影至外部系統，第一版支援 `Create`、health、retry classification（`internal/service/calendar.RetryableError` 區分暫時性與永久性失敗）與 reconciliation（`Service.Reconcile` 回報 `dead_letter` backlog）；不提供 `Busy`、`Update` 或 `Cancel`。

目前唯一的具體實作是 `internal/calendar.SandboxAdapter`——一個不對外發出任何請求的 deterministic fake，用於在真實 OAuth 授權模式核准前先建置並測試 outbox／worker／retry 狀態機（見「Outbox／Calendar delivery」章節）。它不是 Google 或 Microsoft 的正式整合。

## AI Provider Adapter Contract

Provider-neutral port（`internal/service/conversation.AIProvider`）只負責「單輪訊息 → 候選值＋範圍分類」：輸入一則患者訊息、一個 reference time 與目前已知的合法服務代碼列表，輸出候選的 `serviceCode`／日期／時段偏好（早上／下午／晚上）／患者姓名／email／`offeredSlotOrdinal`（患者對上一輪候選時段的**位置**指代，1–5 或 -1 代表最後一個，僅位置指代，不解析確切時間），以及是否屬於超出範圍分類。Port 不做多輪對話記憶、不判斷預約是否合法——這些一律由 `internal/service/conversation` 呼叫既有的 `internal/service/booking` 與 `internal/service/scheduling` 確定性邏輯完成；`offeredSlotOrdinal` 同樣只是候選值，實際是否對應到一個仍可預約的時段，由 `internal/service/conversation` 現查 `GetAvailability` 後才決定是否套用，AI 本身不判斷任何時段是否合法或仍可預約。

第一版由 `internal/ai` 提供唯一的具體實作：一個 OpenAI-compatible Chat Completions HTTP client，要求模型以固定 JSON schema 回傳擷取結果。啟動時必須設定 `AI_PROVIDER_API_KEY`、`AI_PROVIDER_BASE_URL`、`AI_PROVIDER_MODEL`，缺少任一值 `cmd/api` 必須直接啟動失敗，比照 `CLINIC_TIMEZONE`／`DATABASE_URL` 的 fail-closed 慣例，不得使用隱性預設值。呼叫逾時或回傳內容無法解析為預期 JSON schema 時，視為擷取失敗：conversation service 必須 fallback 成一句固定的澄清回覆，不得把例外往上拋成 `500`，也不得套用任何候選值到 session。

**這是本階段的開發／測試用介接選擇，不是根目錄 README「實作前必須決定的事項」第 3 項核准的正式 production AI provider**；正式供應商選型與適用的健康資料／隱私協議仍待診所核准，核准前不得把本 adapter 的預設值當成 production 承諾對外宣稱。

### Voice Transcription Endpoint

`POST /voice/transcriptions`（不需 session id、不需 `If-Match`——轉錄本身無狀態，patient 端須再把回傳文字送進既有 `/booking-sessions/{id}/messages` 才會影響 booking session）接受 `multipart/form-data`，欄位 `audio` 為錄音檔案：

- Request body 上限 **10 MiB**——這是本 endpoint 專屬的例外上限，不是放寬本節開頭記載的全域 64 KiB JSON body 上限；超過回傳 `413 REQUEST_TOO_LARGE`。
- `audio` 的 `Content-Type` 僅接受 `audio/webm`、`audio/ogg`、`audio/mp4`、`audio/wav`、`audio/mpeg` 白名單；不在白名單或缺少檔案回傳 `400 INVALID_REQUEST`。
- 成功回傳 `200`：

  ```json
  { "text": "I'd like to book a cleaning next Tuesday afternoon" }
  ```

- 呼叫底層 AI 轉錄供應商逾時或失敗，回傳 `503 VOICE_TRANSCRIPTION_UNAVAILABLE`——與 §AI Provider Adapter Contract 對話擷取失敗時靜默 fallback 成澄清樣板不同，轉錄端點的唯一產出就是文字本身，失敗時沒有樣板可以代替，因此明確以 503 告知前端改走文字輸入；文字輸入路徑不受影響，任何時候都完整可用。
- 底層實作沿用 `internal/ai` 既有的 OpenAI-compatible provider（同一組 `AI_PROVIDER_BASE_URL`／`AI_PROVIDER_API_KEY`），另外要求 `AI_PROVIDER_TRANSCRIPTION_MODEL` 環境變數，缺少時 `cmd/api` 比照其他 AI/clinic 設定直接啟動失敗，不得使用隱性預設值。轉錄請求固定帶 `language=en`，因患者端僅支援英文。**同樣是本階段開發／測試用介接選擇，非正式核准的 production 供應商**；音訊資料涉及的健康資料／隱私協議與 §AI Provider Adapter Contract 所述情況相同，仍待診所核准。

Google 與 Microsoft 授權模式必須分別在 sandbox 驗證後核准，不預設共用同一 OAuth flow。各 provider 必須使用 least-privilege access，並記錄 credential owner、rotation、revocation、reauthorization 與 tenant/calendar access boundary。

## 安全與維運底線

- Production 必須有 TLS、受管理 secrets、加密 storage/backup、point-in-time recovery、員工 SSO/RBAC 與 audit trail。
- Log、trace、metric 與 alert 不得包含患者訊息、姓名、email、token、Calendar reference 明文或 provider response body。Audit 只使用 entity ID、action、actor ID 與 timestamp。
- 處理患者資料前必須完成適用的隱私/健康資料審查與 vendor agreement。
- 每日監控 outbox backlog、`dead_letter`、provider health 與預約衝突；每季進行 access review、backup restore 與 provider outage drill。

## 待診所確認

1. Clinic IANA timezone、休息時段。營業時間與假日已建立 `clinic_hours`／`clinic_closures` schema（見「Scheduling & Booking Production Data Model」），但實際數值尚待診所提供；資料表可為空，空表時依 fail-closed 規則視為無可預約時段，不得臆測預設值。Slot interval 與最短提前預約時間走必填環境變數，實際數值同樣待診所提供。
2. Google/Microsoft 授權模式與 tenant 權限——outbox 機制已建置並以 `internal/calendar.SandboxAdapter` 這個不對外發出請求的 fake 運作（見「Outbox／Calendar delivery」），真實 OAuth 串接仍待此項核准後才能開始。Credential storage 已初步決定沿用 `professional_calendars.calendar_ref_ciphertext`（application-layer 加密後存 PostgreSQL）；該 table 尚未建立，將於串接真實 provider 時一併新增。
3. Outbox retry interval、最大次數與 backoff——目前以必填環境變數 `CALENDAR_OUTBOX_MAX_ATTEMPTS`、`CALENDAR_OUTBOX_RETRY_BACKOFF_SECONDS`、`CALENDAR_WORKER_POLL_INTERVAL_SECONDS` 表示，缺少時 `cmd/api`／`cmd/calendar-worker` 直接啟動失敗，實際數值仍待診所提供。`dead_letter` 告警對象與人工處理 SLA 仍未定義——`internal/service/calendar.Service.Reconcile` 目前只回報 backlog，不接任何外部通知系統。
4. Session 過期時間（`BOOKING_SESSION_TTL_MINUTES` 實際數值）、availability 查詢範圍與 rate-limit 數值；`appointment_idempotency_keys` 24 小時 retention 的過期清除 job 尚未實作，屬已知限制。

## 後端實作前待釐清

1. ~~BookingSession、Appointment、outbox、idempotency 與 audit 的完整 production schema。~~ 已解決：BookingSession、Appointment、idempotency、audit、`appointment_outbox` 的 schema 已定義於「Scheduling & Booking Production Data Model」。`professional_calendars` schema 已定義但**尚未建立為 migration**——只在串接真實 provider 憑證時才需要。
2. ~~各 API endpoint 的 request/response schema、必填欄位與完整 status/error mapping。~~ 已解決：見「Scheduling／Booking Endpoint Schemas」。
3. ~~所有 provider outbox 狀態組合至 `calendarDelivery` 的完整映射。~~ 已解決：見「Outbox／Calendar delivery」章節的映射規則與 `internal/service/calendar.Service.DeliveryStatus`；`POST /appointments` 回應現在含 `calendarDelivery` 欄位。
