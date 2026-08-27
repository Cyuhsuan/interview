# internal/model/ 作業規範

跨層共用的資料結構。跨分層不變量見 [`../../AGENTS.md`](../../AGENTS.md)；欄位定義見 [`../../README.md`](../../README.md)「Catalog Production Data Model」等章節。

## 邊界

- 只放純資料結構（struct + 明確的 `gorm:"column:...;primaryKey;type:..."` tag + `TableName()` 方法），不依賴 GORM 的隱式命名慣例。
- 不得包含商業邏輯、驗證規則或方法（資格判斷、狀態轉換等一律留在 `internal/service`）；model 只描述資料形狀，不描述行為。
- 欄位型別、constraint 與 nullable 規則必須與 README 對應章節的 schema 表一致；新增欄位前先更新 README，不得讓 model 與 migration/README 三者漂移。
- 自然鍵（非 UUID）的技術表（如 `seed_history`）在 struct 旁加註解說明為何不用 UUID，比照既有 `seed.go` 寫法。
- `handler` 的 API DTO 與這裡的 model 是兩組獨立型別（見 [`../handler/AGENTS.md`](../handler/AGENTS.md)），不得讓 handler 直接序列化 model。
