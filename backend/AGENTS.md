# 後端作業規範

本文件適用於 `backend/`，是跨分層不變量 + 分層文件索引。實作特定功能前，必須先完成 `backend/README.md` 中對應的待釐清 contract。

## 分層文件索引

分層邊界的具體規則（誰能 import 什麼、誰擁有 interface、誰處理 transaction）各自收斂在該層的 `AGENTS.md`，不在本文件重複。修改某一層前，先讀該層文件：

- [`cmd/AGENTS.md`](cmd/AGENTS.md) — composition root 與 CLI。
- [`internal/handler/AGENTS.md`](internal/handler/AGENTS.md) — HTTP 轉換層。
- [`internal/service/AGENTS.md`](internal/service/AGENTS.md) — 商業邏輯層。
- [`internal/repository/AGENTS.md`](internal/repository/AGENTS.md) — PostgreSQL 存取層。
- [`internal/model/AGENTS.md`](internal/model/AGENTS.md) — 共用資料結構。
- [`internal/platform/AGENTS.md`](internal/platform/AGENTS.md) — 跨層基礎設施。
- [`migrations/AGENTS.md`](migrations/AGENTS.md) — schema 變更流程。

## Contract

- `backend/README.md` 是後端架構、資料模型、API 與外部整合的唯一 contract，包含 base path 與版本規則；本文件與各分層文件都不複製 endpoint 或 schema。
- Public behavior 變更必須同步更新 README。未獲診所核准的設定只能列於「待診所確認」，不得以預設值實作。

## 跨分層不變量

以下規則適用於所有層，違反任一層都視為違反 contract：

- PostgreSQL 是預約、人員、服務、availability 與 audit state 的唯一事實來源；Google Calendar 與 Microsoft Outlook 是只接收資料的外部 projection，任何一層都不得讓它們反向影響 availability 或預約判定。防重疊、狀態機與同步流程依 README「PostgreSQL-first 預約一致性」與「防止重疊與狀態」執行；外部同步失敗不得回滾或刪除已 commit 的預約。
- UUID、時間、version、ETag、idempotency、status 與 error code 依 README「Canonical Types」定義，不得在任何一層另創格式。
- Schema 變更必須使用有序、可審閱的 migration，規則見 [`migrations/AGENTS.md`](migrations/AGENTS.md)。
- Calendar、AI、clock、ID generator 與 repository adapter 必須有 contract test 或 deterministic fake。
- AI 模型只能理解語言與擷取候選值；預約是否合法一律由 `internal/service` 的確定性程式碼判斷。

## 驗證

- Unit：資格、時長、營業邊界、假日、半開區間、timezone 與 DST。
- PostgreSQL integration：重疊 constraint、併發確認、idempotency、transaction、migration 與 seeder。
- Adapter contract：throttling、timeout、token expiry、部分成功、重複 delivery、retry、`dead_letter` 與 reconciliation。
- API：限制、ETag、error contract、authorization、CSRF/origin、rate limit 與隱私。
- 交付前執行適用的 format、static analysis、test、build、migration check、secret scan 與 final diff review；未執行的檢查不得宣稱通過。
