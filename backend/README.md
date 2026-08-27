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
| `POST` | `/appointments` | 使用 `If-Match` 與 `Idempotency-Key` 確認預約 |

Appointment body 只含 UUID `bookingSessionId`，不包含 version。Booking session 的 `selectedSlot` 必須含 UUID `professionalId`、RFC 3339 `start`/`end` 與 IANA `timeZone`。

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
| `503` | `AVAILABILITY_UNAVAILABLE` |
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

## Calendar Adapter Contract

Provider-neutral ports 只負責將 PostgreSQL appointment 投影至外部系統，第一版支援 `Create`、health、retry classification 與 reconciliation；不提供 `Busy`、`Update` 或 `Cancel`。

Google 與 Microsoft 授權模式必須分別在 sandbox 驗證後核准，不預設共用同一 OAuth flow。各 provider 必須使用 least-privilege access，並記錄 credential owner、rotation、revocation、reauthorization 與 tenant/calendar access boundary。

## 安全與維運底線

- Production 必須有 TLS、受管理 secrets、加密 storage/backup、point-in-time recovery、員工 SSO/RBAC 與 audit trail。
- Log、trace、metric 與 alert 不得包含患者訊息、姓名、email、token、Calendar reference 明文或 provider response body。Audit 只使用 entity ID、action、actor ID 與 timestamp。
- 處理患者資料前必須完成適用的隱私/健康資料審查與 vendor agreement。
- 每日監控 outbox backlog、`dead_letter`、provider health 與預約衝突；每季進行 access review、backup restore 與 provider outage drill。

## 待診所確認

1. Clinic IANA timezone、營業時間、假日、休息時段、slot interval 與最短提前預約時間。
2. Google/Microsoft 授權模式、tenant 權限與 credential storage。
3. Outbox retry interval、最大次數、`dead_letter` 告警對象與人工處理 SLA。
4. Session 過期時間、availability 查詢範圍、rate-limit 數值與資料 retention/deletion 週期。

## 後端實作前待釐清

下列技術細節必須在加入相關後端程式碼前寫入本 contract 並完成審閱：

1. BookingSession、Appointment、outbox、idempotency 與 audit 的完整 production schema。
2. 各 API endpoint 的 request/response schema、必填欄位與完整 status/error mapping。
3. 所有 provider outbox 狀態組合至 `calendarDelivery` 的完整映射。
