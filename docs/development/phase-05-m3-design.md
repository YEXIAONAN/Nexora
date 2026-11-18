# Phase 05 M3 贡献业务服务设计

日期：2026-11-18。正式授权范围为 Service、Repository、认证 HTTP API、争议与收件确认；不含 Vue、奖金或自动合并。基线 develop / M3 远端分支均为 b336bed96c6fdb57acf94560c7f25358c50ab61e，Schema v7；main 为 36bddd6ed0b45f24bce3f978a3caaa376a511673。已验证 PR #5 合并及 CI 44259268271 四项成功，初始工作区干净。00001–00007 不修改。

## 身份、范围与事务

Handler 只从 reqctx.IdentityFrom 获取 actor，Cookie 仅由后端读取；请求体无 actor/proof_token/session 字段，未知字段拒绝。Service 重新核对数据库 session、有效 Team 与 Project 成员，不使用 ROOT/平台权限豁免。正常操作通过 projects.Service.Within：READ COMMITTED → Team 锁 → 实时成员/授权检查 → Case 锁 → expected_version → v7 helper → 事件/审计 → commit。读操作同样授权。

| 操作 | 范围 |
| --- | --- |
| 项目负责人、有效显式贡献核定人 | 项目案例范围；核定/裁定仍逐版本回避 |
| 其他有效项目成员 | 本人份额、提案或争议相关案例；列表过滤、不可见详情 404 |
| 非项目成员、跨 Team、离队者 | 普通案例/结果读取与写入 404，无平台角色例外 |
| 授权管理 | 负责人授予其他有效成员；负责人自身由其他有效项目成员中的 Team owner/admin 或已授权核定人授予；禁止自授；可本人撤销 |
| 草案与份额 | 负责人/有效显式核定人；受益人不能调整或提交自己的分配；撤回仅提案人 |
| 正式核定 | 显式授权、实时 epoch、非提案/受益人/来源作者/来源审核人/已披露 COI |
| 裁定 | 上述独立性，另排除原核定人与争议当事人 |
| 直接异议 | 有效项目成员；引用具体有效确认，不授予其他 Team 的读取权 |
| 代提异议 | 有编辑权者引用本人经会话核验且绑定原文理由/确认的请求凭据 |

普通成员的本人相关案例不等于仅返回自己的比例；该案例完整分配是其核对与申诉依据。申请人在合法发起异议后成为该案例当事人。角色、任务权重和行为计数均不换算份额。

## SQL 调用清单

| 业务动作 | 数据库调用 |
| --- | --- |
| 唯一建案 | contribution_create_case |
| 授予/撤销 | contribution_set_appraiser |
| 添加/修改/删除草案份额 | contribution_set_share（删除用 0） |
| 提交、撤回、放弃 | contribution_transition |
| 核定 | contribution_confirm |
| 修订 | contribution_revise |
| 发起异议、代提 | contribution_raise_dispute |
| 维持/推翻 | contribution_resolve |
| 草案内容保存 | 在 Team/Case 锁及 expected_version 后仅 UPDATE 最新 draft 的说明/证据，v7 触发器权威校验及记史 |
| COI、告知准备、本人凭据 | scoped INSERT，对应 v7 guard/event/audit 仍是最终防线 |

不在 Go 重建状态机，不直接 UPDATE effective/latest/workflow。Go 的动作许可与预期版本检查用于明确 403/409，数据库继续权威校验转换和完整性。

所有变更有 client_request_id。Team 锁内读取不可变 audit_events 的 contribution.command_completed 回执，按 Team/Project/actor/key 绑定规范化输入 SHA256 与最小结果 ID/revision；相同键相同输入重放，不重复变更，不同输入 409。回执与领域数据同事务，保存服务端 Request ID；不把会话或完整请求存入回执。不新增迁移或另一套状态存储。

## 收件与离队边界

项目内本人结果 GET 只展示已核定的摘要、说明、份额与告知状态，绝不产生凭据。独立 POST 必须 received=true，并携带 case/version/confirmation、已展示摘要、expected_version、client_request_id；身份只来自认证上下文，令牌只来自已认证 HttpOnly Cookie。Service 在事务中重新验证实际收件人和完整关联，把 Cookie 作为瞬时证明交给 v7；令牌不参与输入摘要、响应、日志或审计。收件仅表示收到，不表示同意；旧版本凭据不能满足新确认的归档。

离队者没有普通 Team 数据访问。提供独立的本人争议请求 intake：仅接受本人明确提供的引用和原文理由，用真实 Cookie 核验；只有真实历史项目关系与匹配确认能产生身份请求凭据。统一返回 202 和调用者自己提供的 client_request_id，不返回 Team/案例/核定内容，不声明核验成功；无效引用同样返回接收回执，不产生虚假凭据。授权编辑者只能在正确案例中引用实际存在且摘要一致的凭据代提。

本阶段不向离队者开放核定结果读取或普通收件确认；无可信受控告知时归档保持阻塞。补充的离队送达渠道需产品批准身份、最小披露及投递机制，不能以 M3 旁路恢复离队 Team 访问。该限制不影响其历史份额或受控本人请求的验证。

## API 与错误

OpenAPI 3.1 为请求/响应清单的唯一契约。路径分为项目贡献核定人、contributions 案例/版本/历史/证据/草案/份额/提交/核定/修订/异议/裁定/COI/告知，以及本人结果和不回传业务数据的独立请求 intake。所有写入使用现有 CSRF、Request ID 和统一 envelope。无 Vue 页面；生成 TypeScript 类型仅同步契约。

401 未认证/失效 session；404 不可见/跨域；403 可见但无操作权/利益冲突；409 状态/旧版本/幂等冲突；400 格式、422 字段验证；500 未预期异常。SQLSTATE 显式映射，不把 PostgreSQL 原文、DETAIL 或参数写入客户端或贡献错误日志。数据库故障会使业务、事件、审计和命令回执全部回滚。

## 验证与数据库角色

使用既有 testdb 与 PostgreSQL 17 临时隔离 schema，禁止开发库迁移。Service 测试覆盖授权、原子性、并发和幂等；HTTP 测试通过真实登录 Cookie+CSRF 证明身份绑定、IDOR、COI、失效 session 和显式收件，不以伪造 Identity 替代。持续运行 M2 18 组、Phase 02–04 集成、全量 -race、make verify、既有 E2E；25 项旧 E2E 只证明回归，不代表新贡献 UI 验收。

迁移/DDL 连接与普通应用连接应分离。应用角色不应是 superuser、schema/table/function owner、CREATEROLE/CREATEDB/BYPASSRLS，不能 CREATE、ALTER、TRUNCATE、禁用触发器或改审计；v7 函数为 invoker，执行需明确的业务 DML/SELECT 和函数 EXECUTE。不会自动修改当前运行库的角色或凭据；部署权限检查与受限角色验证记录于最终报告。
