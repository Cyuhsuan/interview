# internal/repository/ 作業規範

PostgreSQL 存取層。跨分層不變量見 [`../../AGENTS.md`](../../AGENTS.md)；資料模型見 [`../../README.md`](../../README.md)。

## 邊界

- 這是唯一允許 import GORM 或持有 `*gorm.DB` 的地方。`*gorm.DB` 一律由 `internal/platform/database` 建立的單例注入，不得在 repository 內自行 `gorm.Open` 或建立第二個 connection pool（`cmd/migrate` 的獨立連線例外，見 [`../../cmd/AGENTS.md`](../../cmd/AGENTS.md)）。
- 只實作 `internal/service` 對應 package 宣告的 interface（ports，見 [`../service/AGENTS.md`](../service/AGENTS.md)），不得包含商業規則——資格判斷、時長計算、availability 邏輯都不屬於這一層。
- `NewRepository(db *gorm.DB) *Repository` 只接受既有單例，不建立新連線。用 `var _ Interface = (*Repository)(nil)` 做編譯期介面斷言。
- `gorm.ErrRecordNotFound` 轉換為 `nil, nil`（not-found 在這一層不是 error，由呼叫方 service 決定如何處理找不到的情況），其餘錯誤一律用 `fmt.Errorf("...: %w", err)` 包裝以保留 context。
- 需要 transaction 的模組（如 `seed`）拆成擁有 `WithTx` 的 `Repository` 與只做 CRUD、不含商業規則的 unexported `txRepository`。
- 查詢邏輯只能寫在這裡；`service`、`handler` 不得直接操作 `*gorm.DB` 或撰寫 SQL。
- Schema 變更（新增欄位、索引、constraint）一律透過 migration，不得用 `AutoMigrate`，見 [`../../migrations/AGENTS.md`](../../migrations/AGENTS.md)。
