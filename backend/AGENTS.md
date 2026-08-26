# 後端作業規範

本文件適用於未來所有位於 `backend/` 下的檔案。

## 未來實作 contract

- 使用 Go，並讓排程規則獨立於 transport、storage、AI 與 Calendar provider。
- AI 可以擷取 intent 與欄位值，但不得核准服務、選擇不合資格的人員或確認可預約狀態。
- Model provider、Google Calendar、Outlook、資料持久化、clock 及 ID generator 都必須具有可測試的介面。
- 提供時段前及確認流程內，都必須讀取外部忙碌時段。
- Production 預約必須使用 database transaction 或同等的 concurrency control。
- Calendar 寫入必須具備冪等性，並透過 durable outbox 傳送及 reconciliation。
- Live availability 失敗時採 fail-closed；Calendar 寫入失敗必須可觀測且可重試。
- 絕不得記錄患者訊息、姓名、email、access token、refresh token 或 Calendar response body。

## API 要求

- 發布前對 public API 進行 versioning。
- 前端開始實作前，先定義 request、success、error、idempotency、authentication 及 rate-limit 行為。
- 後端必須驗證 body 大小、訊息長度、日期、email、時區、服務、專業人員及狀態轉換。
- 回傳穩定的 machine-readable error code，以及對患者安全的簡明英文說明。
- Availability response 不得洩露其他患者的身分或預約細節。

## 安全要求

- 使用 OAuth authorization-code flow、加密 refresh token 及 least-privilege scope。
- 使用受管理的 secrets、TLS、加密 storage/backup、資料保留與刪除控制、audit trail 及員工 RBAC。
- Exact-origin policy、request limit、濫用防護、dependency/container scanning 及去識別化 telemetry。
- 上線前完成適用的隱私與健康資料審查；vendor agreement 必須符合實際傳送的資料。

## 交付前必要驗證

- 每種服務/專業人員組合、時間長度、關門邊界、週末/假日、重疊、時區及 daylight-saving transition 的 unit test。
- 證明兩個同時確認不能預約相同人員與時段的 concurrency test。
- Google 與 Microsoft sandbox tenant 的 adapter contract test，包含 throttling 與 token expiry。
- Idempotency、部分 Calendar 失敗、outbox retry 及 reconciliation 測試。
- API validation、rate limit、authorization、privacy、startup、migration、backup、restore 及 graceful shutdown 檢查。
- Formatting、static analysis、test、build、final diff 及 secret scan。
