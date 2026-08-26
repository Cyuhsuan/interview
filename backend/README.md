# 後端內部架構指南

## 建議架構

初期採用 modular Go monolith，而非 microservices。預約是一個高度重視一致性的流程；對單一診所而言，單一可部署服務更容易進行 transaction、監控、安全管理與維護。透過明確介面，未來仍可在需要時拆分元件。

```text
React client
    |
HTTPS JSON API
    |
Go application
    +-- conversation state machine
    +-- AI intent/value extractor interface
    +-- deterministic scheduling domain
    +-- transactional appointment repository
    +-- durable calendar outbox
    +-- Google Calendar adapter
    `-- Microsoft Graph adapter
             |
      PostgreSQL + worker
```

## Domain 責任

後端負責：

- 服務清單及所需時間。
- 專業人員能力與資格。
- 營業時間、假日、休息時段、最短提前預約時間及 slot interval。
- 內部保留時段與外部忙碌時段。
- 最終可預約判定及衝突檢查。
- 確認、預約參考 ID、idempotency、audit record 及 Calendar delivery state。
- 對話服務範圍及安全拒絕類別。

AI extractor 會回傳 intent、service、date、time、name 與 email 等結構化候選值。所有值都必須由確定性程式碼驗證。若 AI 無法使用或輸出無效，流程必須退回明確提問，不得自行虛構可預約狀態。

## 建議預約 contract

Phase 2 開始時才會正式核准完整 API。預期能力包括：

- 開始或恢復短期 booking session。
- 提交英文訊息或明確的結構化選項。
- 在不包含患者身分的情況下查詢可用時段。
- 預覽暫定選項。
- 使用 idempotency key 確認預約。
- 回傳 confirmed、conflict、provider unavailable、validation、rate limit 及 unexpected failure 等結果。

確認順序：

1. 驗證 session、服務、專業人員、患者欄位及要求的開始時間。
2. 重新取得 Google 與 Outlook 最新的忙碌時段。
3. 若無法驗證必要的可預約資訊，採 fail-closed。
4. 在單一 database transaction 中鎖定專業人員/時間範圍，並建立預約與兩筆 outbox record。
5. 回傳已持久化的預約參考 ID。
6. Worker 以具冪等性的方式建立 Google 與 Outlook 活動；對 throttling 或 transient failure 進行 backoff retry，並對 terminal failure 發出 alert。
7. Reconciliation 比對 provider event 與內部狀態。

## Calendar 整合

每位專業人員應分別擁有 Google Calendar ID 與 Outlook Calendar ID。Provider adapter 至少支援：

- `Busy(from, to, professionals)`
- `Create(appointment, idempotencyKey)`
- `Update(appointment, version)`
- 核准取消階段後的 `Cancel(appointment, reason)`
- Health、throttling、token refresh、retry classification 及 reconciliation ID

App 外部的 Google 設定：

1. 建立診所擁有的 Google Cloud project。
2. 啟用 Google Calendar API。
3. 設定 OAuth consent screen、verified domain、privacy URL 及 redirect URI。
4. 僅要求 free/busy 與建立活動所需的最小 Calendar scope。
5. 若 Google 要求，完成應用程式驗證。
6. 將每位專業人員的行事曆 delegate/share 給診所身分，並記錄 Calendar ID。
7. 將 client credential 與加密後的 refresh token 儲存在 production secret system。

App 外部的 Microsoft 設定：

1. 在 Microsoft Entra 中註冊診所擁有的 application。
2. 設定 redirect URI，以及受管理的 credential 或 certificate。
3. 申請最小必要的 Microsoft Graph Calendar permission，並取得 administrator consent。
4. 若 tenant 控制允許，將 mailbox/Calendar access 限制在三位專業人員。
5. 記錄每個 Outlook Calendar ID，並儲存加密後的 refresh credential。

Google 目前說明，在門檻內的標準 Calendar API 使用不會額外收取 API 費用，並計畫於 2026 年稍晚開始對極高的每日用量計費：[官方 quota 與定價說明](https://developers.google.com/workspace/calendar/api/guides/quota)。Microsoft Graph 會對 Outlook workload 套用 throttling：[官方服務限制](https://learn.microsoft.com/graph/throttling-limits)。

## 安全與隱私計畫

最低 production 控制：

- Edge 使用 TLS；受管理服務間在支援時也使用 TLS。
- 使用受管理的 PostgreSQL，包含 encryption、backup、point-in-time recovery、migration，以及避免重疊預約的 exclusion/locking protection。
- 使用受管理 secrets 及 OAuth refresh-token rotation/revocation；不得在 environment file 中存放長效 access token。
- 資料最小化、明確的保留/刪除政策、患者資料存取流程、regional control，以及適用的健康資料/隱私協議。
- 營運與 audit tool 使用員工 SSO 及 role-based access。
- 隨機 opaque ID、idempotency key、input/body limit、exact-origin rule、rate limit、WAF/bot protection 及安全錯誤訊息。
- Log、trace、metric 與 alert 不得包含患者文字、姓名、email、token 或 provider body。
- 加密 outbox、audit trail、reconciliation、dependency/SBOM scanning、SAST/DAST、restore drill 及 incident response。

適用 HIPAA、GDPR 或其他健康資料/隱私要求，取決於司法管轄區及診所合約。在處理患者資料前，法律與安全負責人必須核准選用的 cloud、AI、speech、Calendar、telemetry 及 support provider。

## Production 成本預估

假設：單一診所、英文 Web channel、三位專業人員、不含付款及既有牙科系統整合；2026 年綜合工程費率為每小時 USD 120–180。

| 工作項目 | 預估成本 |
|---|---:|
| 產品探索、流程、UX 與無障礙 | $10k–$18k |
| Production Go API、PostgreSQL、預約 concurrency 及 outbox | $24k–$42k |
| Google/Microsoft OAuth 與 Calendar onboarding | $12k–$22k |
| React 文字/語音應用程式與 browser QA | $12k–$20k |
| 安全、隱私、可觀測性、CI/CD、負載及復原 | $20k–$38k |
| 診所驗收、上線及 contingency | $12k–$25k |
| **Production 上線總預估** | **約 8–12 週，USD 90k–165k** |

不包含：法律費用、正式認證、第三方滲透測試、vendor BAA 額外費用、電話 channel、診所管理系統整合、資料遷移及 24/7 support。

以下以每月 3,000 次對話及 600 筆完成預約為規劃案例：

| 每月項目 | 精簡方案 | 高可用 production |
|---|---:|---:|
| Compute 與 edge | $20 | $120 |
| 受管理 PostgreSQL 與 backup | $40 | $180 |
| Queue/cache/outbox worker | $0 | $60 |
| Monitoring、log、WAF 與 secrets | $25 | $150 |
| 文字 AI extraction | <$5 | $20 |
| Domain、email 與其他服務 | $15 | $70 |
| 工程維護 | $960 | $4,320 |
| **預估總額** | **約 $1.1k/月** | **約 $4.9k/月** |

在此用量下，單純基礎設施約為每月 $100–$600；工程維護通常是較大的持續成本。AI 成本取決於供應商、prompt、model 及資料保留條款。採購前必須重新確認各部署區域的價格。

## 維護計畫

- 每日：監控預約錯誤、provider health、outbox backlog 及 reconciliation alert。
- 每週：透過去識別化 telemetry 檢查異常拒絕/fallback 比例及 Calendar failure。
- 每月：更新 dependency 與 base image、檢查成本/容量、依政策輪替 credential，並抽查診所驗收流程。
- 每季：檢查 access、執行 backup restore、測試 token revocation/provider outage、審查 AI eval 與 vendor pricing，並確認 retention deletion。
- 營業時間、服務、model、permission 或 Calendar identity 變更後：重新執行資格、衝突、服務範圍、邀請、無障礙及復原驗收測試。
