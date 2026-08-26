# 前端作業規範

本文件適用於 `frontend/` 下所有檔案。

## 實作 contract

- 使用 React 與 TypeScript。
- 患者端文案必須使用簡明英文。
- 文字預約流程是主要路徑，即使沒有麥克風權限或語音支援也必須完整運作。
- 語音控制必須清楚呈現正在聆聽、已停止、不支援、權限遭拒及重試等狀態。
- 不得把 AI provider 或行事曆 credential 放在瀏覽器中。
- 服務資格、可預約狀態、時間長度及確認結果一律以後端回應為準。
- 在後端回傳已確認的預約 ID 前，不得以 optimistic UI 顯示預約成功。
- 保留 session context，但不得將非必要的患者資料長期存放於瀏覽器 storage。

## 必須支援的使用者狀態

- 初始服務選擇。
- 日期與可預約時間選擇。
- 蒐集患者姓名與行事曆邀請 email。
- 預覽及明確確認。
- 成功狀態，包含日期、時間、服務、專業人員及參考 ID。
- Loading、無可用時段、輸入無效、時段剛被預約、Calendar 傳送延遲、rate limit、離線及非預期錯誤。
- 超出服務範圍時，提供清楚的診所或緊急服務聯絡方式。

## 最低品質標準

- 從 320 px 起支援響應式版面。
- 支援鍵盤操作、清楚的 focus、語意化標題、有 label 的控制項、live region 通知及適當對比。
- 尊重 reduced-motion 與使用者音訊偏好。
- 不得只依賴顏色、動畫或語音傳達狀態。
- 測試目前版本的 Chrome、Safari、Firefox 與 Edge；語音差異必須記錄，不得隱藏。
- 醫療與隱私文案必須客觀。避免不實保證或宣傳式內容。

## 交付前必要驗證

- Typecheck、lint、unit test 與 production build。
- 完整文字預約流程及重要錯誤狀態的 component test。
- 僅鍵盤操作與螢幕閱讀器 smoke test。
- 行動裝置 viewport review 及瀏覽器語音支援矩陣。
- Production bundle 不得含有 secrets、患者資料、debug log 或外部 script。
