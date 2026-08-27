# internal/platform/ 作業規範

跨層基礎設施（`config`、`database`、`httpproblem`、`httpserver`、`idgen`）。跨分層不變量見 [`../../AGENTS.md`](../../AGENTS.md)；技術棧邊界見 [`../../README.md`](../../README.md)「技術棧」。

## 邊界

- `database`：唯一建立 `*gorm.DB` 的地方，透過 Fx lifecycle（`OnStart` ping、`OnStop` close）管理單例連線池。其他任何 package（包含 `internal/repository`）都不得自行建立第二個連線池，只能接受注入。
- `config`：`Load()` 是所有環境變數讀取的唯一入口；`CLINIC_TIMEZONE`（需通過 `time.LoadLocation` 驗證）與 `DATABASE_URL` 缺少或無效時必須回傳 error 讓啟動失敗，不得用隱性預設值放行。`godotenv.Load()` 只能在 `APP_ENV != "production"` 時呼叫。
- `httpproblem`：所有 error response 必須經過這裡的 `Write`／`WriteInternal`，格式對應 README「Error Contract」。`WriteInternal` 不得把 `err.Error()` 洩漏進回應 body；新增 error code 常數時同步核對 README 的 HTTP status 對照表，不得自創未列在 README 的 code。
- `httpserver`：`NewEngine()`／`RegisterServer()` 只做 Gin engine 組裝與 Fx lifecycle 綁定的 listen/shutdown，不掛載任何業務路由——路由註冊留給各模組的 `internal/handler/*/RegisterRoutes`。
- `idgen`：所有 entity ID 產生的唯一入口，底層必須是 CSPRNG（如 `google/uuid` 的 `NewRandom()`）；不得在其他 package 另行產生 UUID 或使用非 CSPRNG 來源。
- 本目錄下每個子套件單一職責，不得互相 import 對方的內部細節；跨套件共用時透過 `config.Config` 顯式傳遞。
