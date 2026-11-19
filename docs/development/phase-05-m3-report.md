# Phase 05 M3 贡献业务服务开发与验收报告

日期：2026-11-18。交付分支：`feature/phase05-m3-contribution-service`。M3 实现 Service、Repository、认证 HTTP API、完整业务权限、争议和显式收件确认；没有开发 Vue、奖金、消息中心、邮件通知或 M4，不自动合并。

## 1. 基线、授权与工程纪律

产品负责人通过本次正式任务授权 M3；M1 的 18 项 Accepted 决策保持不变，金额仍 Proposed。fetch 核实 `origin/develop` 与远端 M3 分支均为 `b336bed96c6fdb57acf94560c7f25358c50ab61e`，main 为 `36bddd6ed0b45f24bce3f978a3caaa376a511673`。已通过连接器核实 M2 PR [#5](https://github.com/YEXIAONAN/Nexora/pull/5) 合并，合并后 CI [44259268271](https://github.com/YEXIAONAN/Nexora/actions/runs/44259268271) 四项成功。

开始时工作区干净，从远端现有 M3 分支建立跟踪分支；本地 develop/main 未改。已审查 M1/M2 设计、ADR 0007、v7 完整性与八个辅助函数、M2 集成测试、项目/成果 Service 和 Handler、认证与 reqctx、中间件、路由及 OpenAPI。00001–00007 没有任何修改；Schema 保持 v7，未添加迁移或第二套状态机。没有对开发数据库执行迁移/角色变更，也没有重启或清理开发栈。

| 里程碑 | 提交 |
| --- | --- |
| M3.1 契约、读取/权限矩阵与风险 | `9f98c4932b2884b7c9452e817a677d1be3395f8f` |
| M3.2 DTO、作用域 Repository 与 v7 调用 | `4c23ca0f5e0a5e8c56a0da35e3255b2c4726d93f` |
| M3.3 身份、权限、原子命令与幂等 | `70ee6ea0d564bbad404e31fe85204df9f36e442e` |
| M3.4 HTTP、真实 Cookie 收件、生产连接防线 | `fe5d4f965ea5bc4312500b03a16640c7b6fc38b3` |
| M3.5 综合测试、凭据证据查询、能力一致性、报告与文档 | `8653ac454604eb1af886ff455603c47d03fb503f` |

评审 PR：[\#6](https://github.com/YEXIAONAN/Nexora/pull/6)，目标 develop；已核实 open、非 draft、未合并。代码提交 `8653ac454604eb1af886ff455603c47d03fb503f` 的 [push CI 44288031845](https://github.com/YEXIAONAN/Nexora/actions/runs/44288031845) 与 [PR CI 44289841751](https://github.com/YEXIAONAN/Nexora/actions/runs/44289841751) 均 Success，逐项核对 Backend (Go)、Frontend (Vue)、Secret scan、Containers 四项成功。后端 CI 实际执行 `go test -race -count=1 -tags=integration ./...`。

本次评审证据补记只更新文档，不改变以上已验证代码。报告维护提交的完整 SHA 及其最新 CI 由交付回复和 [PR 当前 Checks](https://github.com/YEXIAONAN/Nexora/pull/6/checks) 记录；不以旧 head 的 CI 冒充新 head。

## 2. 模块结构与边界

| 文件 | 职责 |
| --- | --- |
| `apps/api/internal/contributions/model.go` | 明确的输入/输出 DTO，整数基点、版本、确认、争议、凭据和能力；不返回会话内部字段 |
| `repository.go` | scoped 查询、八个受控函数、最新/有效读取、不可变回执查询、凭据安全投影 |
| `service.go` | 当前会话绑定、Team/Project/Case 授权、COI、预期版本、命令幂等和同事务审计 |
| `delivery.go` | 告知准备、本人结果、独立收件 POST、无 Team 数据回传的本人请求 intake、授权人员凭据核对 |
| `validation.go` / `errors.go` | 输入规范化、范围/长度/JSON 校验、SHA256 输入摘要、明确且脱敏的 SQLSTATE 映射 |
| `handler.go` | reqctx 身份、UUID/JSON、HttpOnly Cookie、统一 envelope；通过 RouteModule 挂接认证路由 |
| `apps/api/internal/database/application_role.go` | 生产启动的只读数据库角色权限检查，不改变 ACL |

继续使用 chi、pgx、项目 Service.Within、READ COMMITTED、Team→Case 锁、Request ID、CSRF 和审计。所有八个 helper 原样复用：create_case、set_appraiser、set_share、transition、confirm、revise、raise_dispute、resolve（均带 contribution_ 前缀）。Go 不直接写 latest/effective/workflow；仅最新 draft 的说明/证据 UPDATE、COI/告知/凭据 INSERT 仍由 v7 触发器权威校验并自动记史。

## 3. API 清单

契约是 [OpenAPI 3.1](../api/openapi.yaml)，生成结果为 `apps/web/src/api/schema.d.ts`。以下项目路径相对于 `/api/v1/teams/{teamID}/projects/{projectID}`；共 29 个新操作、24 个路径。

| 方法 | 路径 | 行为 |
| --- | --- | --- |
| GET / POST | `/contribution-appraisers` | 查询 / 显式授权（原因、授权 expected_version、client_request_id） |
| DELETE | `/contribution-appraisers/{userID}` | 撤销授权 |
| GET | `/contributions/eligibility` | 本人的显式授权资格；不能替代具体案例 COI 判定 |
| GET | `/contributions/pending` | 本人确实可独立核定的当前案例 |
| GET / POST | `/contributions` | 授权范围内案例列表 / 真实 review 幂等建案 |
| GET | `/contributions/{caseID}` | 最新工作版本、旧有效确认、具体操作能力分离 |
| GET | `/contributions/{caseID}/versions` | 所有版本及确认事实 |
| GET | `/contributions/{caseID}/history` | 版本、事件、调整、COI、争议、裁定与告知历史 |
| GET | `/contributions/{caseID}/dispute-requests/{identityRequestID}` | 授权人员查询实际本人请求凭据，不返回会话 |
| GET | `/contributions/{caseID}/evidence` | 真实当前正式验收、成果、审核及安全附件引用 |
| PATCH | `/contributions/{caseID}/draft` | 草案说明与证据快照 |
| PUT / DELETE | `/contributions/{caseID}/shares/{userID}` | 手工设置 / 删除份额；无角色或权重换算 |
| POST | `/contributions/{caseID}/submit` | 完整草案提交待核定 |
| POST | `/contributions/{caseID}/withdraw` | 提案人撤回待核定版本 |
| POST | `/contributions/{caseID}/abandon` | 放弃草案，不能替代核算 |
| POST | `/contributions/{caseID}/confirm` | 真正独立核定 |
| POST | `/contributions/{caseID}/revisions` | 建立修订，不覆盖旧有效结果 |
| GET / POST | `/contributions/{caseID}/disputes` | 争议/证据/裁定查询；本人或经验证代提 |
| GET | `/contributions/{caseID}/dispute-requests/{identityRequestID}` | 授权人员核对实际本人声明、版本、摘要与服务端核验时间；无会话字段 |
| POST | `/contributions/{caseID}/disputes/{disputeID}/resolutions` | 独立维持或原子推翻 |
| POST | `/contributions/{caseID}/conflicts` | 追加本人或授权记录的直接利益冲突 |
| GET / POST | `/contributions/{caseID}/notices` | 告知及实际收件状态 / 准备告知（不是送达） |
| POST | `/contributions/{caseID}/notices/{noticeID}/receipt` | 本人明确收到指定核定结果，不表示同意 |
| GET | `/contribution-results/mine` | 查询本人结果，不产生收件凭据 |
| POST | `/api/v1/contribution-dispute-requests`（完整路径） | 本人请求 intake，固定返回自己的接收回执，不返回核验结果或 Team 数据 |

写请求不得包含 actor_id、confirmed_by、proof_token 或可写工作流/有效指针。输入未知字段拒绝，多个 JSON 对象拒绝；整数基点只接受整数。Draft 可不完整；提交/核定由数据库重检 10000 和证据。实例路由要求 Schema v7，显式迁移仍由运维执行。

## 4. 身份、权限、IDOR 与 COI

Handler 只使用 `reqctx.IdentityFrom`；每个 p_actor 都是该 identity.UserID。Service 在同一 Team 锁内检查真实 sessions 的 ID/User/撤销/过期、有效 Team 与 Project 关系，然后检查作用域与动作资格。reqctx 的平台标记不会升级业务权限。

| 角色/状态 | 实施规则 |
| --- | --- |
| 项目负责人 / active 显式核定人 | 项目贡献读取；符合动作与回避条件时整理草案 |
| 其他有效项目成员 | 仅本人相关案例（份额、提案、确认/裁定、已记录直接利益或争议当事关系）；不能枚举其他案例 |
| Team owner/admin、ROOT、平台管理员 | 必须拥有实际业务关系；不自动读取、授权、核定或裁定 |
| 核定人 | active Team+Project、显式授权/epoch、非受益人/提案人/来源作者/任务审核人/已知 COI |
| 裁定人 | 以上独立资格，另排除原核定者及争议当事人；不允许矛盾有效裁定 |
| 离队或项目移除 | 普通贡献读取/收件/操作 404；历史事实保留，重新加入不恢复旧核定授权 |

“本人相关”不会因普通 request intake 本身变成案例读取授权。合格项目成员可以发起合法争议并成为当事人；代提必须由授权人员引用实际存在且同 subject/version/confirmation/理由摘要的凭据，不接受自由文本作为核验成功。

不可见资源 404；可见但无操作权或 COI 403；未登录、失效/过期/撤销会话 401；状态、旧版本或不同内容重用幂等键 409；格式 400、字段 422。23503/23505/23514/40001/40P01/22xxx 明确映射，未知异常 500。贡献错误不记录 PostgreSQL 消息、DETAIL、查询或凭据参数；保留的冲突原因只有 SQLSTATE，供重试分类及测试拒绝死锁。

## 5. 显式收件与真实凭据

1. 授权人员创建 authenticated_ack 告知，绑定实际份额持有人、正式版本和 confirmation；只产生准备记录。
2. 本人通过认证 GET 查看摘要、说明、份额和确认时间；GET 不写任何收件事实。
3. 独立 POST 必须 received=true，绑定 case/version/confirmation、展示的内容摘要、expected_version 与 client_request_id。
4. 后端从认证 HttpOnly Cookie 读取令牌，核对 session 与当前身份，再在同一事务交由 v7 核验持有证明；令牌不进入 JSON 输入、回执摘要、响应或审计。
5. v7 清空瞬时 proof_token，仅保留会话引用、本人、摘要、渠道和实际服务端时间。对外 DTO 不返回 session_id、token_hash 或 proof_token。
6. 其他用户、错误摘要、旧 expected_version、缺乏明确动作均不能确认；旧版本凭据不满足新正式版本归档。

**收件表示收到，不表示同意，也不会解决争议。** 无实际收件时，归档继续阻塞；登录成功、创建通知、自由文本 evidence_ref/digest/时间不作送达。

安全测试还复现并修复：归档冲突必须在贡献资源可见性/操作授权之后判断，避免无项目权限者收到 409 代替 404；争议中的草案不得提示可提交，归档不得提示可编辑。两者均保持 v7 权威约束，不修改治理政策。

另复现并修复现有 Request ID 反射路径：X-Request-Id 不能采用请求中的 Cookie 值。否则原始凭据可能被回显并流入审计。先有失败测试，再以通用中间件最小防护修复，并用真实登录 Cookie 的收件 POST 核验响应、日志和持久表均不含令牌。

## 6. 离队本人请求与剩余送达边界

intake 是本人明确提交声明的受理，不是“已收到核定结果”。正确的历史项目关系、当前有效确认、真实 Cookie 和精确声明可生成不可变 dispute_request 凭据。无效/不可见引用同样返回 202 + 调用者自己提供的 request_id，不制造凭据，也不泄露是否存在案例或核验成功。授权人员可以在正确案例中读取实际安全投影再代提；正式争议展示可核对的声明与核验时间，不展示会话。

当前不向离队者开放普通 Team 结果 GET 或 receipt POST。若其在离队前未收件，项目保持归档阻塞；需要产品补充批准受控告知、最小披露与投递通道，不能在 M4 用普通接口恢复其 Team 数据访问或后台代收件。该限制是正式任务允许保留的安全边界，不回填历史、不制造送达。历史份额和经验证本人请求照常保留。

## 7. 原子性、有效版本和幂等

Service 不重建状态机。八个 helper 决定状态转换，确认/替代/推翻及后继 Draft、不可变事件和审计仍在同一事务。case/latest/effective 在响应中分离；`undisputed_confirmed` 只从 v7 effective-shares 视图派生：未决争议 false，未确认修订不替代旧有效结果。

每次写入都有 client_request_id，命令回执复用 append-only audit_events，绑定 Team/Project/actor/key、规范化内容 SHA256 和最小结果 ID/revision。持有 Team 锁时先检查实时权限，再查询重放，随后 expected_version、领域变更和回执同事务完成。相同键/内容返回原结果，不同内容 409；撤销后不能借旧回执恢复权限。没有独立提交的短暂错误指针，也没有提交成功但只写部分数据。

## 8. 数据库应用角色

生产 API 增加只读启动检查，拒绝迁移 owner、超级用户/高权限成员资格、数据库/业务 schema CREATE、当前业务 schema 的对象所有权、TRUNCATE、audit UPDATE/DELETE，以及 SET session_replication_role 的授权。此检查不改任何角色或 ACL，不是 RLS；普通业务连接与迁移连接必须分离。

真实 PostgreSQL 测试使用独立 LOGIN 角色及随机密码建立新连接，不用 owner 会话 SET ROLE 伪装：核定、收件、真实登录均成功；DDL、禁用触发器、replica 模式、TRUNCATE、修改审计/确认被 42501 拒绝。额外授予审计 UPDATE、schema CREATE 或 TRUNCATE 时，生产检查能拒绝。部署授权与用途见 [应用数据库角色](../operations/application-database-role.md)。当前开发 Compose 的初始化角色没有被擅自更改，不能把它直接当生产应用身份。

## 9. PostgreSQL、安全与回归证据

使用已有 testdb、PostgreSQL 17.11、独立 nexora_test schema 和专用 tmpfs 容器（127.0.0.1:5433）；测试后检查隔离 schema 与临时角色均为 0。未连接开发 PostgreSQL 5432。E2E 只在专用 nexora_e2e、API 8081 和 Vite 5173 上执行。

| 检查 | 结果 |
| --- | --- |
| 全 Go `-race -count=1 -tags=integration ./...` | 通过；contributions 247.864s，database 204.313s，全部被测包成功 |
| M2 18 组数据库测试及迁移/回滚 | 持续通过；旧迁移没有修改 |
| M3 模块 16 个顶层测试组，包含并发子场景 | 通过；最终凭据读取补测见交付记录 |
| `make verify` | 通过：格式/vet、Go、构建、OpenAPI 类型一致性、扫描；Vitest 15 文件 / 76 项，扫描器 8 项自测 |
| Phase 02–04 E2E | 25/25 通过（约 2.2m）；仅旧流程回归，不冒充新贡献 UI 验收 |
| 受限 LOGIN/权限和凭据补测 | 通过；角色测试 9.879s，新增凭据查询后整个贡献包 -race 补测通过；最终包含归档授权/能力检查的整包补测 92.269s |

HTTP 用真实登录获取 Cookie 与 CSRF，覆盖：匿名、未知/过期/撤销 session、CSRF 缺失、actor Body/URL/Header 伪造、跨 Team/Project IDOR、ROOT/平台越权、无授权、自授、受益人/提案人/来源审核人/直接 COI 回避、成员移除及 epoch、真实 rejected/V2 accepted review、确认后不可改、latest/effective/无争议统计分离、归档缺收件拒绝、本人显式收件、他人代收件拒绝、离队请求的无数据回传与授权核验。

真实多连接 Service 测试先观察该隔离 schema 的 pg_stat_activity Lock 等待，再释放阻塞 Team 锁；覆盖唯一建案、相同键重试、并发份额编辑、双核定、撤销/项目退出/Team 退出的双顺序、核定与异议双顺序、异议与裁定双顺序、修订/修订确认与归档。40P01 一律使测试失败，未用 Mock。

审计故障注入发生在原生变更之后的 Go 命令回执阶段：核定、收件、推翻分别抛错，13 张贡献/审计表有序 JSON SHA256 完全不变。旧有效结果、后继草案、事件、凭据与回执均无部分写入。会话令牌在响应、日志、凭据/通知/事件/审计/sessions 持久 JSON 中均未出现。

日志在忽略的 `.cache/phase05-m3-{integration,domain-final,concurrency,role-final,verify,e2e}.log`；可通过现有 test-db.sh + TEST_DATABASE_URL（仅本地读取临时凭据，不能打印）复现 `NEXORA_ENV=test NEXORA_TEST_DB_DISPOSABLE=1 go -C apps/api test -race -count=1 -tags=integration ./...`。只清理本次专用测试容器与临时凭据。

## 10. 已知风险与 M4 待办

- M4 尚未授权；需要基于这些 API 实现页面、明确“收件≠同意”、显示有效/最新/争议状态及能力，不在前端推断权限或份额。
- 离队者的结果告知/收件通道仍需产品补充设计；在此之前归档阻塞，不允许假证据或平台兜底。
- statement/外部 evidence_refs 是编辑者整理的未核验声明/佐证，不证明本人提交、签名或事实真伪；事实判断和未申报 COI 仍需人工治理。
- 当前未实现完整通知、邮件、奖金、跨任务指数、排名或完整统计 API；不可使用最新 Draft 作为正式统计源。
- 数据库角色检查不是 RLS，受限凭据属于可信后端；生产需要单独配置合规应用凭据，不能直接使用开发初始化 owner。没有执行生产部署或数据库变更。
- 幂等回执复用现有审计，Team 锁提供串行化；大规模历史读取/审计键检索的索引与分页优化需真实负载证据后追加迁移，不能擅自改 v7。
- 尚待正式代码评审。没有宣称外部独立审计、正式发布或新增贡献 Vue E2E 已完成；不合并 PR、不进入 M4。
