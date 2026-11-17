# Phase 04 浏览器证据

2026-11-18，`make e2e` 使用独立 nexora_e2e 数据库、真实 API 和 Chrome，Phase 02/03/04 合计 25/25 通过。Phase 04 用例在真实 UI 提交 V1、独立审核驳回、补正提交 V2、审核通过，并再次读取服务器任务状态和进度为 100%。图片仅含本次自动化创建的测试数据。

- [完成与历史版本](completed.png)
- [深色](dark.png)
- [390px 手机宽度](mobile.png)：自动断言无页面水平溢出。

截图由 apps/web/e2e/phase04.spec.ts 生成，未用 Mock 或手工设置审核状态。没有提交 Cookie、密码或浏览器 trace。
