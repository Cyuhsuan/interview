# internal/handler/ 作業規範

HTTP 轉換層。跨分層不變量見 [`../../AGENTS.md`](../../AGENTS.md)；API contract 見 [`../../README.md`](../../README.md)。

## 邊界

- 只能 import `gin-gonic/gin` 與 `internal/platform`；不得 import GORM、不得持有 `*gorm.DB`、不得直接呼叫 `internal/repository`。所有資料存取一律透過該模組的 `service`。
- `handler` 與 `repository` 不得互相依賴，必須經過 `service`（見 [`../service/AGENTS.md`](../service/AGENTS.md)）。
- 只做 request/response 轉換與輸入格式驗證；不得包含商業規則（資格、時長、availability、狀態轉換判斷一律留在 service）。
- Response 使用 handler 自訂的 private DTO struct + 明確 `json` tag（camelCase），不得直接回傳 `internal/model` 的結構——model 是持久層資料結構，不是 API 契約。
- Service 回傳的 sentinel error 用 `errors.Is` 轉換為 `internal/platform/httpproblem` 的錯誤回應；`detail` 欄位依 README「Error Contract」，不得洩漏底層錯誤訊息、SQL 或 stack trace。
- 每個模組（如 `catalog`）的 `RegisterRoutes(engine *gin.Engine, h *Handler)` 掛載在 `/api/v1` group 下；命名與既有 `catalog`、`health` 模組一致。
