# internal/service/ 作業規範

商業邏輯層。跨分層不變量見 [`../../AGENTS.md`](../../AGENTS.md)；業務規則見 [`../../README.md`](../../README.md)。

## 邊界

- 不得 import Gin 或 GORM，不得持有 `*gorm.DB`，也不得 import `internal/repository` 的具體型別。
- 服務資格、時長、availability、狀態轉換與預約合法性判斷一律在這一層完成——這是 README「AI 只能擷取 intent…判定必須由確定性 service 層完成」的落地位置。
- 採 **ports** 慣例：service 自己宣告所需的 repository interface（例如 `ServiceRepository`、`ProfessionalRepository`），由 `internal/repository` 實作；service 只依賴自己宣告的 interface，不依賴 repository 的具體 struct。新增 repository 依賴時，先在 service package 定義 interface，再讓對應 repository 提供實作並在該處加上 `var _ Interface = (*Repository)(nil)` 編譯期斷言。
- 需要 transaction 邊界時，用 `Repository.WithTx(ctx, fn func(TxRepository) error) error` 模式（見 `internal/service/seed`），不在 service 外部組裝 transaction。
- 錯誤一律用具名 sentinel error（如 `ErrInvalidServiceCode`）或實作 `Unwrap()` 的 typed error，讓 handler 能用 `errors.Is`／`errors.As` 判斷；不得回傳裸字串或未分類的 `fmt.Errorf`。
- 常數（regex pattern、限制值等）與其對應的 README 章節在註解中互相對應，避免與 contract 各自漂移。
