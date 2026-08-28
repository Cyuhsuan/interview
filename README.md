# 牙科預約客服 Bot

產品文件基準已完成；專案目前進入 API contract 與 production data model 階段。

## 產品摘要

患者可透過文字或語音選擇標準牙科服務、尋找具備資格且有足夠空檔的專業人員，並確認預約。系統只以 PostgreSQL 判斷 availability，確認後再將活動非同步寫入 Google Calendar 與 Outlook。AI 負責理解自然語言；服務資格、所需時間與預約合法性由後端規則控制。

外部患者不需註冊帳號或登入；每次預約時自行提供姓名與自己的 email，僅作為該筆預約的聯絡與行事曆邀請對象，不建立長期帳號或跨預約識別。

## 診所模型

| 代碼 | 英文服務名稱 | 時間 | 可執行人員 |
|---|---|---:|---|
| A | Service A | 60 分鐘 | Junior、Senior 1、Senior 2 |
| B | Service B | 60 分鐘 | Junior、Senior 1、Senior 2 |
| C | Service C | 150 分鐘 | Senior 1、Senior 2 |
| D | Service D | 120 分鐘 | Senior 1、Senior 2 |
| E | Service E | 360 分鐘 | Senior 1、Senior 2 |

以上名稱、時長與資格為固定值。診所時區、營業時間、假日、休息時段、slot interval 與最短提前預約時間尚待診所確認，不得使用隱性預設值。

## 範圍

本階段規劃範圍：

- 使用 React 建立響應式 Web 應用程式。
- Go 後端 API 與確定性的排程 domain。
- 英文文字對話。
- 瀏覽器語音輸入與語音回覆，並永久保留文字輸入作為 fallback。
- 不綁定特定供應商的 AI 意圖與欄位擷取。
- 將 PostgreSQL 中已確認的預約非同步投影至 Google Calendar 與 Outlook，不讀取外部忙碌時段。
- 清楚的服務範圍界線與安全的診所轉接機制。
- Production 架構、安全、維運與使用文件。

第一版不包含：

- 診斷、檢傷、處方、緊急醫療、保險、治療報價或付款。
- 任何自動取消或改期；相關要求由診所人工處理。
- 電話/PSTN channel、多語言或牙科診所管理系統整合。
- 除非另行核准，否則不包含員工管理後台。

## 文件索引

- [前端產品與客戶指南](frontend/README.md)：患者體驗、語音行為、介面狀態、無障礙、安裝前置條件與診所驗收。
- [前端實作規範](frontend/AGENTS.md)：後續 React 階段的限制與品質要求。
- [後端架構與內部指南](backend/README.md)：架構、預約 contract、Calendar/AI 邊界、安全與 production 維運。
- [後端實作規範](backend/AGENTS.md)：後續 Go 階段的限制與驗證要求。

## 建議交付階段

| 階段 | 交付內容 | 完成條件 |
|---|---|---|
| 1 — Contract | 完整 API schema、production data model 與 Calendar delivery mapping | 跨層 contract 已審閱，可據以實作與測試。 |
| 2 — 後端 | Go API、domain 測試、資料持久化、AI 與 Calendar adapter | 服務資格、時間、衝突、失敗情境及範圍界線測試通過。 |
| 3 — 前端 | React 文字/語音應用程式 | 響應式、鍵盤、螢幕閱讀器、文字輸入及支援瀏覽器的語音檢查通過。 |
| 4 — 整合 | Google 與 Microsoft sandbox 連線 | 每筆已確認預約在各 provider 最多建立一個活動；同步失敗不改變預約狀態。 |
| 5 — Production | 受管理的基礎設施、OAuth、可觀測性、隱私與復原控制 | 通過安全審查、還原測試、診所驗收與 release checklist。 |

## 實作前必須決定的事項

1. 確認診所時區、營業時間、假日、休息時段、slot interval 與最短提前預約時間。
2. 確認 Google／Microsoft 授權模式、tenant 權限與 credential storage；每位啟用的專業人員必須同時具有兩個 provider 的獨立 mapping。
3. 選擇 production AI provider，以及適用的健康資料與隱私協議。（目前後端已用 provider-neutral 介面接上一個開發／測試用的 OpenAI-compatible adapter，見 `backend/README.md`「AI Provider Adapter Contract」；這不等於本項決策已核准，正式供應商與健康資料/隱私協議仍待診所確認。）
4. 定義資料 retention/deletion、緊急情況、取消／改期轉接與員工支援政策。
