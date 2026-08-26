# 後端作業規範

本文件適用於 `backend/` 下所有檔案。Repository 目前仍在文件階段；使用者未明確啟動實作前，不得加入 Go 程式碼、migration、seed 檔、套件清單、產生檔、部署設定或依賴。

## Source of Truth

- `backend/README.md` 是後端架構、canonical types、data model、REST API、PostgreSQL/Calendar 一致性與 seeder 的唯一 contract。
- 修改 public behavior 時必須同步更新該 contract，不得在本文件複製 endpoint 或 schema 規格。
- 診所尚未核准的項目必須留在 README 的「待診所確認」，不得在 production 使用隱性預設值。

## 不可違反的規則

- Domain 必須獨立於 HTTP、PostgreSQL、AI 與 Calendar provider；外部依賴一律置於 application-owned interface 之後。
- AI 只能產生候選 intent/value，不得核准服務、人員資格、availability 或 appointment。
- PostgreSQL 是唯一事實來源。所有寫入必須先 commit PostgreSQL，再透過 durable outbox 非同步寫入 Google/Microsoft；request transaction 內禁止寫入外部 Calendar。
- 確認前必須重新讀取 Google 與 Microsoft busy intervals。任一必要 provider 不可用時 fail-closed，不建立 appointment。
- 外部同步失敗不得回滾、刪除或降級已 commit 的 appointment；必須 retry、reconcile，窮盡後告警人工處理。
- Appointment 重疊必須由 PostgreSQL exclusion constraint 阻擋，不得只依賴 application pre-check。Appointment、outbox、idempotency record 與 audit record 必須在同一 transaction 建立。
- UUID、時間、version、ETag、idempotency、status 與 error code 必須遵守 README canonical contract，不得另行發明格式。
- 不得記錄患者訊息、姓名、email、access/refresh token、Calendar reference 明文或 provider response body。

## 變更規則

- Public API 一律置於 `/api/v1`。Breaking change 必須建立新 API version，不得靜默改變既有 contract。
- Schema 變更必須使用有序、可審閱的 migration；不得在 API startup 自動 migration 或 seed。
- Migration 必須考慮既有資料、rollback/forward-fix、lock duration 與 rolling deployment compatibility。產生檔不得手動編輯。
- Calendar、AI、clock、ID generator 與 repository adapter 必須有 contract test 或 deterministic fake。

## 交付前必要驗證

- Unit tests：所有合格與不合格的服務/人員組合、時長、營業邊界、假日、半開區間、timezone 與 DST。
- PostgreSQL integration tests：重疊 exclusion constraint、兩個併發確認、idempotency、transaction rollback、migration 與 seeder。
- Adapter contract tests：Google/Microsoft throttling、timeout、token expiry、部分成功、重複 delivery、retry、`dead_letter` 與 reconciliation。
- API tests：body/field limit、ETag/`If-Match`、error contract、authorization、CSRF/origin、rate limit 與隱私。
- 交付前必須完成 formatting、static analysis、test、build、migration check、secret scan 與 final diff review；未執行的檢查不得宣稱通過。
