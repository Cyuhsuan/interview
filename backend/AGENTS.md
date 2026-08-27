# 後端作業規範

本文件適用於 `backend/`。實作特定功能前，必須先完成 `backend/README.md` 中對應的待釐清 contract。

## Contract

- `backend/README.md` 是後端架構、資料模型、API 與外部整合的唯一 contract，包含 base path 與版本規則；本文件不複製 endpoint 或 schema。
- Public behavior 變更必須同步更新 README。未獲診所核准的設定只能列於「待診所確認」，不得以預設值實作。

## 實作限制

- 分層為 handler / service / repository，不採 DDD。`handler` 只做 HTTP request/response 轉換並呼叫 `service`；`service` 持有商業邏輯與其依賴的 interface；`repository` 只實作該 interface 並操作 PostgreSQL，不得包含商業規則。`handler` 與 `repository` 不得互相依賴，必須透過 `service`。技術棧（Gin/GORM/Fx/godotenv）與各套件的使用邊界見 README「技術棧」，不得跨層使用。
- PostgreSQL 一致性、防重疊、AI 邊界與 Calendar 同步流程一律依 README「PostgreSQL-first 預約一致性」與「防止重疊與狀態」執行；不得在 application 層繞過 exclusion constraint，也不得因同步失敗回滾或刪除已 commit 的預約。
- UUID、時間、version、ETag、idempotency、status 與 error code 依 README「Canonical Types」定義，不得另創格式。
- Schema 變更必須使用有序、可審閱的 migration，並考慮既有資料、rollback/forward-fix、lock duration 與 rolling deployment compatibility；產生檔不得手動編輯，不得在 API startup 自動 migration 或 seed。
- Calendar、AI、clock、ID generator 與 repository adapter 必須有 contract test 或 deterministic fake。

## 驗證

- Unit：資格、時長、營業邊界、假日、半開區間、timezone 與 DST。
- PostgreSQL integration：重疊 constraint、併發確認、idempotency、transaction、migration 與 seeder。
- Adapter contract：throttling、timeout、token expiry、部分成功、重複 delivery、retry、`dead_letter` 與 reconciliation。
- API：限制、ETag、error contract、authorization、CSRF/origin、rate limit 與隱私。
- 交付前執行適用的 format、static analysis、test、build、migration check、secret scan 與 final diff review；未執行的檢查不得宣稱通過。
