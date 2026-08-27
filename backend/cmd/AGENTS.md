# cmd/ 作業規範

Composition root。跨分層不變量見 [`../AGENTS.md`](../AGENTS.md)；架構與 contract 見 [`../README.md`](../README.md)。

## 邊界

- `go.uber.org/fx` 只能在這裡使用（`cmd/api`、`cmd/calendar-worker`）；`internal/` 下的 business package 不得 import Fx。新模組上線時，在對應 `cmd/*/main.go` 的 `fx.Provide` 依 repository → interface adapter → service → handler 的順序注入，比照 `cmd/api/main.go` 既有寫法；`fx.Invoke` 只用來強制建構單例（如 DB pool ping）與註冊路由/server。
- `cmd/migrate` 是獨立 CLI，不經過 Fx 或 `internal/platform/httpserver`；它自建 `gorm.Open` 連線，因為 migration/seed 是一次性維運操作，不共用 API process 的 DB pool 生命週期。
- Migration 與 seed 不得在 `cmd/api` 啟動時自動呼叫；只能透過 `cmd/migrate` 明確執行。
- Production 環境不得依賴 `godotenv`；缺少必要環境變數時，`cmd/api` 必須直接啟動失敗，不得使用隱性預設值。
