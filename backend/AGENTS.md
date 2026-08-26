# 後端作業規範

本文件適用於 `backend/`。Repository 仍在文件階段；使用者未明確啟動實作前，只能修改文件。

## Contract

- `backend/README.md` 是後端架構、資料模型、API 與外部整合的唯一 contract；本文件不複製 endpoint 或 schema。
- Public behavior 變更必須同步更新 README。未獲診所核准的設定只能列於「待診所確認」，不得以預設值實作。
- Public API 使用 `/api/v1`；breaking change 必須建立新版本。

## 實作限制

- Domain 不得依賴 HTTP、SQL、AI SDK 或 Calendar SDK；外部依賴置於 application-owned interface 後方。
- AI 只提供 intent/value 候選；資格、時長、availability 與預約合法性由 deterministic domain code 判斷。
- PostgreSQL 是唯一事實來源；預約、outbox、idempotency 與 audit 必須同一 transaction 寫入，外部 Calendar 寫入只能在 commit 後非同步執行。
- 最終確認只重新檢查 PostgreSQL；外部 Calendar 不得作為 availability 輸入。已 commit 預約不得因同步失敗而回滾或刪除。
- 重疊必須由 PostgreSQL exclusion constraint 阻擋，不可只做 application pre-check。
- UUID、時間、version、ETag、idempotency、status 與 error code 依 README 定義，不得另創格式。
- Schema 變更必須使用有序、可審閱的 migration；不得在 API startup 自動 migration 或 seed。
- Migration 必須考慮既有資料、rollback/forward-fix、lock duration 與 rolling deployment compatibility；產生檔不得手動編輯。
- Calendar、AI、clock、ID generator 與 repository adapter 必須有 contract test 或 deterministic fake。

## 驗證

- Unit：資格、時長、營業邊界、假日、半開區間、timezone 與 DST。
- PostgreSQL integration：重疊 constraint、併發確認、idempotency、transaction、migration 與 seeder。
- Adapter contract：throttling、timeout、token expiry、部分成功、重複 delivery、retry、`dead_letter` 與 reconciliation。
- API：限制、ETag、error contract、authorization、CSRF/origin、rate limit 與隱私。
- 交付前執行適用的 format、static analysis、test、build、migration check、secret scan 與 final diff review；未執行的檢查不得宣稱通過。
