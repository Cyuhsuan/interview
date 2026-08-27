# migrations/ 作業規範

Schema 變更流程。跨分層不變量見 [`../AGENTS.md`](../AGENTS.md)；schema 定義見 [`../README.md`](../README.md)「Catalog Production Data Model」。

## 邊界

- 使用 `golang-migrate/migrate/v4` 的 `{6 位數序號}_{description}.{up|down}.sql` 命名（例：`000001_create_catalog_tables.up.sql`）；序號遞增且不重用，`up`／`down` 必須成對存在。
- 產生後的 migration 檔不得手動編輯；schema 再變更一律新增下一號 migration，不修改既有檔案（既有檔案已可能在其他環境執行過）。
- 禁止 GORM `AutoMigrate`；所有 schema 變更（含 constraint、index、default）只能經由這裡的 SQL migration。
- 不得在 `cmd/api` 啟動時自動執行 migration 或 seed；只能透過 `cmd/migrate` 明確指令觸發（見 [`../cmd/AGENTS.md`](../cmd/AGENTS.md)）。
- 每個 migration 需考慮既有資料相容性、rollback／forward-fix 路徑、lock duration 與 rolling deployment（新舊程式碼同時跑）相容性，不得假設變更當下沒有流量。
- Constraint（NOT NULL、CHECK、exclusion constraint、FK ON DELETE 行為）必須在 SQL 層建立，不得只靠 Go 層驗證替代。
