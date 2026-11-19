# Phase 05 M4 贡献管理前端开发与验收报告

日期：2026-11-19（Asia/Shanghai）。分支：`feature/phase05-m4-contribution-workspace`。交付范围为 Vue 3 + TypeScript 贡献工作台、M3 API 客户端、真实收件、独立核定、争议/修订及测试文档。等待正式代码评审，不合并，不进入 M5。

## 1. 基线与工程纪律

开工先 fetch 并核实 M3 PR [#6](https://github.com/YEXIAONAN/Nexora/pull/6) 已合并，远端 develop 和 M4 起点均为 `d030198f5c520bfc2d653b4546100ab08f91868e`。[合并后 develop CI 44461565234](https://github.com/YEXIAONAN/Nexora/actions/runs/44461565234) 的 Backend (Go)、Frontend (Vue)、Secret scan、Containers 均 completed/success 后才开始代码开发。原工作区干净，从远端建立 M4 跟踪分支；main/develop 的本地及远端分支均未修改。

Schema 保持 v7；`00001–00007`、Go 后端、OpenAPI 和生成 schema 类型无改动。未升级或清理开发数据库，未改开发 Compose 栈。没有生产 Mock、贡献算法、奖金金额、完整消息中心或新离队结果通道。所有 API 行为取自现有 OpenAPI 3.1，不新增猜测字段。

## 2. 页面、组件和状态逻辑

| 文件（相对 apps/web/src） | 职责 |
| --- | --- |
| `views/ProjectContributionsView.vue` | 已验收待建案、案例状态筛选、最新/有效指针分离、工作台入口 |
| `views/ContributionCaseView.vue` | 案例组合、服务端能力、明确核定确认、争议与独立裁定、告知、冲突、修订 |
| `views/ContributionAppraisalView.vue` | 资格查询、本人可独立核定待办、授权与撤销 |
| `views/MyContributionResultsView.vue` | 当前 Team 可访问项目本人结果、具体版本摘要、真实收件与独立 POST |
| `views/ContributionDisputeIntakeView.vue` | 本人已持有标识的无数据披露请求受理 |
| `components/contributions/ContributionDraftEditor.vue` | 人工基点、份额编辑/删除、分类证据与草案说明 |
| `ContributionEvidencePanel.vue` | 正式成果、审核、附件和安全引用 |
| `ContributionVersionPanel.vue` | 份额、状态、具体版本和不可变核定摘要 |
| `ContributionHistoryPanel.vue` | 全部历史版本、事件和份额调整，不抹除旧确认 |
| `ContributionProxyDispute.vue` | 授权查询真实本人请求凭据，按原文及准确身份/版本代提 |
| `components/contributions/contributionPresentation.ts` | 精确十进制字符转整数基点、状态中文、错误和可信收件校验 |
| `composables/useContributionResource.ts` | AbortController、generation、身份/Team/Project 隔离、重复写保护、409 重新读取 |
| `api/contributions.ts` | 29 个契约类型化操作，复用唯一 requestJSON 客户端 |

项目详情仅增加明确入口，业务没有塞入原单文件。路由按需加载，导航支持本人结果和异议受理；顶部 Team 切换同步切换贡献上下文。复用现有浅灰/白与克制绿色样式、中文语义标签、原生表单/键盘、错误和空状态。

## 3. API 交互清单

项目路径基于 `/api/v1/teams/{teamID}/projects/{projectID}`，案例路径基于其 `/contributions/{caseID}`。

| 方法及路径 | 前端用途 |
| --- | --- |
| GET/POST `/contribution-appraisers`，DELETE `/{userID}` | 核定人授权事实、手工授权与撤销 |
| GET `/contributions/eligibility`，GET `/contributions/pending` | 本人资格和服务端独立待办 |
| GET/POST `/contributions`，GET 案例 | 列表、真实验收建案、latest/effective/permissions |
| GET 案例 `/versions`、`/history`、`/evidence` | 具体版本、完整历史、成果证据 |
| PATCH `/draft`，PUT/DELETE `/shares/{userID}` | 说明/证据与人工整数份额 |
| POST `/submit`、`/withdraw`、`/abandon` | 草案工作流；不把放弃当核算完成 |
| POST `/confirm`、`/revisions` | 明确独立核定和保留旧有效版本的修订 |
| GET/POST `/disputes`，POST `/disputes/{disputeID}/resolutions` | 本人/验证代提异议，独立维持或推翻 |
| POST `/conflicts` | 直接利益冲突的不可变补充记录 |
| GET/POST `/notices` | 历史告知/实际收件状态与准备告知 |
| POST `/notices/{noticeID}/receipt` | 唯一本人显式收件入口 |
| GET `/contribution-results/mine` | 无副作用本人结果 |
| GET `/dispute-requests/{identityRequestID}` | 授权人员在正确案例核对实际本人凭据 |
| POST `/api/v1/contribution-dispute-requests` | 无 Team 数据回传的本人声明受理 |

所有包装函数接收 AbortSignal，类型来自生成的 `components['schemas']`。写入按各自契约携带 `client_request_id` 及适用的 `expected_version`；新建案例/intake 没有契约不存在的版本字段。复用 Cookie credentials 和 `X-Nexora-CSRF`，不读取 HttpOnly Cookie，不引入第二客户端。

收件发送正在展示的 `case_version_id`、`confirmation_id`、`content_digest`、case revision、新请求 ID 与 `received=true`。只有 notice、具体 confirmation、recipient、digest 和真实服务端时间全部匹配的 authenticated receipt 才显示收到。409 后重新读取并清除勾选，不自动覆盖或再次提交；GET 不调用 receipt。

## 4. 权限、竞态和历史

操作按钮来自服务端 case permissions/dispute.can_resolve，待办来自服务端独立筛选。授权 UI 使用服务端 Project/Team/eligibility 事实，对照现有 SetAppraiser 规则；所有操作最终仍由后端校验。确认和裁定历史只有查看，不提供编辑入口。

每次身份/Team/Project 切换同步清空状态、使旧 generation 失效并 abort；即使底层请求无法撤销，迟到结果和写入回流也不能刷新新上下文。共享 requestJSON 在解析响应体后先检查取消状态，防止旧 401 的已取消 JSON 读取误触发当前会话退出；该问题已用先失败后通过的回归测试复现。顶部 Team 已切换而路由尚在等待时也立即隐藏旧数据；异步团队列表到达后重新对齐深链接，避免冷启动页面永久空白。写期间禁用提交，同一案例冲突仅重新读取。

Draft/待核定均不作正式贡献；未确认修订保留旧有效指针。未决争议单独提示，推翻、替代和修订分别使用不同中文语义。版本、核定与裁定历史始终可展开。

离队者普通结果及案例查询继续 404；intake 只显示本人请求受理编号，不显示项目或核验成功。获授权代提必须引用服务端实际凭据及原文；不能用自由文本制造核验或送达。缺少可信收件时归档继续阻塞。

## 5. 测试与验收

所有本地门禁在最终前端源码冻结后执行成功：

| 检查 | 最终结果 |
| --- | --- |
| `pnpm typecheck`、`pnpm lint`、`pnpm format:check` | 全部通过 |
| `pnpm test` | 25 文件 / 206 项通过；M3 基线 76 项，M4 新增 130 项 |
| `pnpm build` | 通过，Vue 类型与 Vite 生产构建成功 |
| `pnpm api:check` | OpenAPI types match the contract；生成类型/契约文件均未改 |
| Go 全量 `-race -count=1 -tags=integration ./...` | 真实 PG17.11 全包通过；contributions 158.511s、database 127.402s、httpapi 89.968s |
| `make verify` | 全部通过，包括 Go 单元/vet/build、前端各门禁、契约与扫描器 8 项自测 |
| Playwright 全量 `make e2e` | 31/31 通过，约 1.5m；原 Phase 02–04 25 项与 M4 新增 6 项 |
| 桌面/手机/键盘 | 1440px 工作区、390px 本人结果及案例无水平溢出；Space 勾选、Tab/Enter 显式收件 |
| 作用域与清理 | 仅专用 PG loopback 5433 / nexora_test、nexora_e2e；临时 schema、角色和 E2E 库均为 0 |

新增 Vitest 分布：API 36、隔离组合式 12、基点/状态/凭据 38、草案/版本 9、案例 12、经核验代提 5、附件权限提示 1、核定授权 5、本人结果 9、隐私 intake 2、旧取消 401 防回流 1，共 130。旧 76 项保持通过。使用可控 Promise 检查迟到读取、迟到写入、Team/Project/身份切换、冷启动深链接与 409；不靠 sleep 假造竞态。

六条真实 E2E 串联实际成果作者、成果审核人、非受益提案人、独立核定人、独立裁定人：

1. 真实登录、项目/成员建立、上传实际附件、成果提交和独立正式验收。
2. 由真实 review 建案，人工输入 100.00% 并验证发送 `share_bps=10000`；草案 effective=null；提案人及受益人前端无核定按钮、直接 POST 均 403。
3. 明确核定授权；核定人原任务附件 GET 404，负责人通过原有审核人 UI 明确授权后 GET 200 且字节与上传原文相同，实际成果审核人未改变且 `can_confirm=true`。正式核定不可改（PATCH 409）；非收件人 receipt 403；缺实际收件时归档 409。本人 GET/刷新产生 0 次 receipt POST，核对展示版本/核定/摘要后通过键盘独立确认，服务端记录真实 receipt。
4. 本人异议保留原因，原核定人不得裁定（UI 隐藏、POST 403）；独立裁定人推翻，原确认保留、effective 清空，后继草案产生。
5. V2/V3 后继草案份额均为空，重新逐人手填证据和 10000 基点。修订期间 latest=0bps、旧 effective=10000bps；新独立确认后旧版 superseded，V1 overturned 与原始核定事实仍存在；旧收件不能满足新有效版本归档（409）。
6. 已持有真实标识的成员离队后，普通 case/mine GET 404、本人结果页无卡片；intake 只返回 202 + 自己的 received/request_id，UI 不显示核验成功或项目数据。

截图及生成说明见 [浏览器证据](evidence/phase05-m4/README.md)。完整日志保留于忽略目录 `.cache/phase05-m4-integration.log`、`.cache/phase05-m4-verify-final.log`、`.cache/phase05-m4-e2e.log`；不提交 Cookie、口令、会话或 trace。早期定位运行中发现的测试状态码/空后继草案假设均已纠正，最终以完整 31/31 的冻结源码运行作为验收证据，不拿部分或中途 HMR 的运行冒充通过。

专用测试容器由本会话创建（2026-11-19 13:05:35 +0800），最终使用 test-db.sh stop 只删除此容器与其私有临时凭据。清理前核实 `nexora_it_*` schema=0、实际 `nexora_m3_test_*` 角色=0、其他非内置意外角色=0、nexora_e2e 库=0。没有 prune 或开发数据库迁移。

## 6. 提交与评审

| 交付 | SHA |
| --- | --- |
| API、类型、隔离状态及基础测试 | `911d454bc458ac353988b86f7a356844e862c422` |
| 已验收建案及人工草案组件 | `6f83f78f10b119a63c8d57290eb0aad88be847ea` |
| 独立核定、争议、历史与修订 | `75886d1e5e4f88c9fb0e99e3a517f8bac5445dbc` |
| 本人结果、显式收件、隐私受理及导航 | `f0f9678883e40278cd0585b8ae173c02ac317179` |

评审 PR：[\#7](https://github.com/YEXIAONAN/Nexora/pull/7)，目标 develop，不自动合并。最终测试/验收提交 SHA 记录在交付回复及 [PR 提交列表](https://github.com/YEXIAONAN/Nexora/pull/7/commits)，避免文档自引用 SHA。最终 head 的 push 和 pull_request 四项 CI 是交付门禁；实际 run 链接与逐项成功状态记录在 [PR 说明及当前 Checks](https://github.com/YEXIAONAN/Nexora/pull/7/checks) 和交付回复，不用旧 head 的 CI 冒充最终提交。

## 7. 剩余产品限制

- 本人结果依照 M3 契约仅列出为本人准备的告知；核定不自动生成消息或收件事实。没有邮件、完整通知中心和发送服务。
- 离队人员送达仍待产品批准最小披露及可信通道；不允许普通 Team 查询后门或后台代收件。当前缺凭据继续阻塞归档。
- 人工贡献说明、声明、外部引用不证明事实或本人签署；未知利益冲突仍依赖业务人员诚实申报与治理。
- 原任务附件下载继续使用原有任务读取权限，独立贡献核定授权不会自动增加附件读取权。负责人可通过现有审核人管理明确授权读取；未经授权保持拒绝。界面提示先取得必要权限并核对证据，不自动授权、不添加披露通道。
- 没有奖金、金额、排名、跨任务综合指数、自动份额、离队结果门户。没有改变 M1–M3 治理政策。
- 当前列表及团队内本人结果聚合使用既有未分页 API。大量项目/历史下的分页和批量性能优化需要后续契约设计；没有在 M4 添加数据库迁移。
- Playwright 在本地隔离真实栈执行；现有 GitHub 四项 CI 验证格式/类型/单元/构建、Go 集成和容器，不将其冒充云端 Playwright 执行。

操作步骤见 [贡献工作台使用说明](phase-05-m4-workspace.md)。
