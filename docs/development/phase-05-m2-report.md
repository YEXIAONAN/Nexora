# Phase 05 M2：Schema v7 验收报告

日期：2026-11-18（Asia/Shanghai）。交付范围仅 M2；停留在 `feature/phase05-contributions`，未推送、未创建/合并 PR，未推进 M3。产品负责人已明确“批准 M1，启动 M2”，18 项治理规则及附加门禁的真实批准记录见 [ADR 0007](../architecture/adr/0007-contribution-governance.md) 与 [设计 §18](phase-05-design.md#18-2026-11-18-批准记录与-m2-实施澄清)。ADR 0005 的金额部分仍 Proposed。

## 1. 基线、工作区与提交

- 开始时 fetch 核实 `origin/develop = 07f6fa1c201a2257e768a0ab24b27fb8804081f0`、`origin/main = 36bddd6ed0b45f24bce3f978a3caaa376a511673`，本地基线为 develop / Schema v6。Phase 04 合并 CI `44202649641` 四项成功。
- 初始 `git status --porcelain` 中 7 份已修改文档与未跟踪的 M1 设计均保留；原始 patch、状态、设计及摘要备份于被忽略的 `.cache/phase05-m2-preflight/`。未发现不相关修改；未丢弃或覆盖任何用户变更。
- 从确认的 develop 基线创建指定功能分支，M1 文档单独提交。未修改 main/develop；00001–00006 无差异。未运行 reset、clean、force push、全局 Docker 清理或当前开发库迁移。

| 提交类别 | 提交 |
| --- | --- |
| M1 文档及批准记录 | `33b5dab` |
| v7 表、作用域与基础索引 | `d0d2a54` |
| 约束、状态机、事务及最小兼容 | `a2987dc` |
| PostgreSQL 集成、迁移及并发测试 | `30a8e3e` |
| 本报告及文档状态同步 | 本报告所在提交；完整最终 SHA 由交付回复记录 |

本地没有单独命名的“贡献治理最终整改报告”，未虚构该文件；已逐行读取 M1 设计，并核对其整改章节、PRD、ADR 0004/0005、Phase 03/04 设计、00004–00006、数据库/权限矩阵及现有归档实现。批准前原稿中的放弃/送达/旧有效版本统计/版本唯一约束冲突，以正式任务的治理意图为准，实施选择完整保留在设计 §18。

## 2. 数据结构与索引

唯一新增迁移：[00007_contributions.sql](../../apps/api/internal/database/migrations/00007_contributions.sql)。未对现有数据进行贡献建案、成员补造、旧归档回填或状态变更。

| 新增表 | 用途与不可变边界 |
| --- | --- |
| `contribution_appraisers` | 显式项目核定授权；绑定成员 authorization_version，授权版本控制，自动审计 |
| `contribution_cases` | 唯一真实 review 建案、最新/有效指针、权威流程状态及 revision |
| `contribution_case_versions` | 有序版本、提案、说明、证据快照及冻结摘要；允许受控生命周期变化 |
| `contribution_shares` | 整数基点、本人声明、人工依据、分类证据引用及真实历史项目成员 |
| `contribution_share_adjustments` | 自动追加调整前后完整值、原因、操作者、request_id |
| `contribution_confirmations` | 永久追加的正式核定事实、内容摘要和核定时成员快照 |
| `contribution_conflicts` | 追加式直接利益冲突声明 |
| `contribution_disputes` | 具体版本/确认的争议事实及代提身份凭据；禁止 UPDATE/DELETE |
| `contribution_dispute_resolutions` | 每争议最多一条裁定；独立裁定者、理由及审计关联 |
| `contribution_case_events` | 数据库自动追加的变更事件，同事务写 audit_events |
| `contribution_notifications` | 追加式结果告知准备记录；自由文本、摘要及声称发送时间不等于送达 |
| `contribution_attestations` | 经既有会话凭据核验的收件确认或本人代提请求；原始令牌不落库 |

除项目级授权表外，全部案例业务表均有 Team / Project / Task / Case 作用域；版本相关记录进一步具有 scoped Version 引用。UUID、project_members、authorization_version、sessions、audit_events 均复用既有模型。

10 个显式索引（另有主键、唯一约束自动索引）：

| 索引 | 用途 |
| --- | --- |
| `contribution_one_working_version` | 部分唯一：一案例仅一个 draft/pending_review |
| `contribution_cases_project_status_idx` | 项目案例状态读取 |
| `contribution_versions_history_idx` | 案例版本倒序历史 |
| `contribution_shares_user_idx` | 项目成员逐案例份额 |
| `contribution_disputes_case_idx` | 案例争议历史 |
| `contribution_events_case_idx` | 案例事件时间线 |
| `contribution_adjustments_version_idx` | 版本调整前后值 |
| `contribution_conflicts_case_idx` | 案例成员 COI 检查 |
| `contribution_notices_recipient_idx` | 收件对象与核定关联 |
| `contribution_receipts_version_idx` | 具体核定版本及对象的凭据 |

## 3. 复合引用及数据库不变量

真实建案同时关联 tasks、task_submission_versions、task_reviews 的基础表复合键。触发器验证 completed、current_submission_version_id、accepted_review_id 和 approve 决定，提交前延迟约束再次验证。`review_id UNIQUE` 与事务辅助命令共同保证并发唯一幂等；没有对 VIEW 声明外键。

Case→Version 的 latest/effective、Version→裁定的内部退役依据使用延迟复合外键；其余版本、份额、确认、争议、裁定、收件人与身份请求保持完整作用域关联。跨行合计、实时授权和指针一致性由事务触发器/延迟约束完成，普通 CHECK 只处理枚举、整数范围、JSON 类型等行内条件。

- Draft 可空缺或只填写部分份额；份额采用 1–10000 整数，删除成员份额表示移除分配项，不采用浮点。
- 提交 pending_review 与正式核定分别校验总和 10000、非空人工依据及分类证据来源/引用。证据真实性与适当性仍由 M3 流程和独立核定者判断。
- 提交生成冻结摘要；pending 内容冻结，只能由提案人受控撤回 Draft。confirmed 内容、份额、证据及 confirmation 永久不可改。
- latest 为工作版本，effective 为正式有效版本，workflow 由数据库派生；客户端直接写指针、状态或 counter 会拒绝。提交末延迟触发器核对完整最终状态。
- 新修订保留旧有效核定；新版本成功确认后才在同事务将旧版本 superseded、切换有效指针。superseded 表示被后来正式确认替代，overturned 表示经争议裁定推翻。
- 所有版本/份额/确认/争议/裁定/送达凭据变更自动写事件和审计；审计失败即整笔回滚。授权改变同事务写审计。事实、历史、确认、调整、裁定和告知记录禁止 UPDATE/DELETE。

## 4. 核定资格与历史成员

核定人必须是有效 Team + Project 成员，持有 active 显式授权，且授权绑定当前 project_members.authorization_version。项目移除后重新加入产生新 epoch，旧授权保持失效，必须重新显式授权。Team 离队也立即失去核定资格。

数据库拒绝直接受益人、提案人、来源成果作者、来源任务审核人和已记录直接 COI 者核定。项目负责人可给其他有效项目成员授权；负责人自身须由其他具备批准规则所列授权操作权的有效项目成员授予，禁止自授。Team owner/admin、ROOT、平台管理员身份本身不产生贡献核定权。无人合格时保持 pending_review。

贡献份额引用真实 project_members 历史行；离队成员可保留份额与核定时 historical 快照，不要求重新 active。物理删除成员关系受阻，沿用既有软移除；没有补造虚假成员，不将 task_collaborators 当作历史参与真值。旧的最后 owner 约束语义与错误提示保持兼容。

这些约束校验数据库中可验证的身份、资格和 COI。辅助函数参数 actor_id 不是会话认证；M3 Service 仍须认证调用者、绑定 actor、检查完整 Team/Project 权限和请求范围。普通应用必须遵守事务辅助命令和锁顺序；数据库 owner 的 DDL/触发器禁用能力不属于业务防线保证。

## 5. 有效版本、争议与裁定事务

争议只关联当前有效确认的具体 Version + Confirmation，允许有效项目成员本人发起。离队者的代提需要本人有效会话证明及绑定同一确认、同一理由 SHA256 的身份请求凭据；自由文本核验引用不能替代凭据，也不授予离队者 Team 数据读取权限。

原核定者、受益人、提案人、来源成果/审核人、争议本人/代提对象及已知 COI 不能独立裁定。无独立裁定人保留未决；每条 dispute 的唯一 resolution 防止同一争议出现矛盾结论。

uphold 不改变原始份额。overturn 在一个事务内追加裁定，将旧有效版本标为 overturned、清空有效指针、退役已有未确认修订并保留内容、创建指向被推翻版本的新 Draft，以及写事件与审计。原 confirmation 永久保留。若同一确认有多个未决争议，剩余争议仍保持未决；后续不得 uphold 恢复已推翻结果，后续 overturn 追加裁定但不重复创建后继草案。全部争议解决且新版本重新核定前，不恢复正式统计。

## 6. 归档和可核查送达

归档触发器从项目全部基础表真实验收证据出发 LEFT JOIN 案例，不仅检查已经创建的案例。因此无案例、仅 Draft、初始 abandoned 未核算、pending、未决争议、未完成必要修订或缺少真实送达凭据均拒绝；现有项目归档 Service 将该约束映射为 409 conflict。

放弃未确认修订可以结束该工作草案，但不能替代成果的正式核算；只有仍存在旧有效核定、无未决争议且该有效版本送达完整时才满足归档。旧 archived 项目不回填、不重开；归档后全部普通贡献写入拒绝，历史可读。

结果告知采用最小的 authenticated_ack 凭据基础，不包含 Phase 07 通知系统：

1. 告知记录绑定真实份额持有人、具体 Version + Confirmation、渠道、准备人及 request_id。manual_unverified 或单纯“通知创建”一直未满足。
2. 收件人明确确认该核定内容摘要，提交其既有有效会话令牌作为瞬时持有证明。数据库验证 token_hash、subject、未撤销、未过期，并锁定会话防止与撤销交错。
3. 只保留真实 session_id、subject/actor、核定摘要、渠道、服务端 verified_at/sent_at/acknowledged_at、request_id。原始令牌在 BEFORE INSERT 清空并由 CHECK 禁止持久化；凭据不可改。
4. 此渠道以经身份核验的收件确认时刻作为保守送达时间；不把任意人工发送时间、ref 或 digest 当作证明，不等同于收件人同意分配。

没有发送服务或贡献 API，当前真实业务不会自动产生此凭据，缺失时归档仍阻塞。M3 必须提供真实结果展示和明确的本人收件确认流程；不得后台代用户确认或把登录本身当作结果送达。测试中的真实令牌由测试账号登录取得，只用于隔离夹具，不向开发数据制造送达事实。

## 7. 安全读取基础

| 视图 | 边界 |
| --- | --- |
| `contribution_effective_shares` | 只读有效 confirmed + 不可变 confirmation，排除全部未决争议；修订期间仍显示旧有效份额并标记 revising |
| `contribution_version_history` | 逐版本状态、确认事实、latest/effective 标记；区分 superseded / overturned / abandoned |
| `contribution_member_coverage` | 真实项目成员历史、验收成果作者与正式绑定附件上传者的覆盖基础，不用当前 collaborators 推断历史 |
| `contribution_project_counts` | 案例总数、有效确认案例数、待核定数、修订数、未决争议数及推翻/被替代版本数 |

没有完整统计 API；不将 latest 当正式来源。没有任务权重加权、跨任务综合指数、奖金或成员综合排名。视图均携带作用域，M3 查询仍须按授权 Team/Project 过滤；这些视图未引入 RLS。

## 8. 锁顺序与事务调用契约

全部贡献事务辅助命令在 READ COMMITTED 下先锁 Team，再锁 Case，随后读取实时权限并检查 expected_version（即 contribution_cases.revision；授权使用 contribution_appraisers.version，首次为 0）。拒绝 NULL/stale expected_version。所有写保护及归档拒绝使用 REPEATABLE READ / SERIALIZABLE 旧快照执行贡献变更。

| SQL 辅助命令 | 职责 |
| --- | --- |
| `contribution_create_case` | 从真实批准记录唯一幂等建案及初始 Draft |
| `contribution_set_appraiser` | 显式授权/撤销与授权版本校验 |
| `contribution_set_share` | 最新 Draft 手工份额与证据调整，自动前后值及事件审计 |
| `contribution_transition` | 提交/撤回/放弃的受控生命周期 |
| `contribution_confirm` | 独立核定、冻结事实及原子有效指针切换 |
| `contribution_revise` | 创建后继 Draft，保留旧有效结果 |
| `contribution_raise_dispute` | 绑定具体确认的本人/经核验代提争议 |
| `contribution_resolve` | 独立裁定及原子推翻/后继草案 |

成员资格变更使用相同 Team 锁；归档锁 Team 后按 Case UUID 确定顺序锁定案例。事务内自动生成事件和审计，不由调用者分步提交。M3 应调用这些命令，先获取 Team 锁，再操作成员/项目或案例；任意绕过此契约、反向获取行锁的原始 DML 不在无死锁保证内。

## 9. PostgreSQL、迁移与并发验证

使用现有 testdb：仅 disposable 标志 + NEXORA_ENV=test + loopback + nexora_test* / nexora_e2e*，测试逐个独立 schema。PostgreSQL **17.11**，专用临时容器 nexora-devtest-pg / 127.0.0.1:5433 / tmpfs；未向正在运行的 5432 开发库执行迁移。E2E 仅单独创建/删除 nexora_e2e，API 8081、前端 5173。

测试实现：[贡献集成测试](../../apps/api/internal/database/contributions_integration_test.go)、[迁移测试](../../apps/api/internal/database/migrate_integration_test.go)。夹具通过现有 Auth/Team/Project/Task/Submission Service 产生真实用户、成员、附件、提交和 approve/reject 审核，不用 SQL 伪造正式验收状态。

| 验证项 | 结果 |
| --- | --- |
| v6 → v7、空库安装、空库 7 → 0 → 7 | 通过 |
| v6 Phase 04 实际数据升级/空 P5 down，19 张旧表有序 JSON SHA256 前后比较 | 通过 |
| 旧归档项目保持原样，禁止普通回填 | 通过 |
| 正式贡献证据存在时 down 拒绝，版本及全部贡献/审计快照无变化 | 通过 |
| 18 组贡献数据库测试 + 单独 v7 安装/回滚测试 | 通过；最终完整 database 包 -race 补测 61.826s |
| 全 Go PostgreSQL 集成测试，-race -count=1 | 通过；database 包 128.135s，全部被测包成功 |
| make verify（lint/vet、Go/Vitest、构建、契约、secret scan） | 通过；15 个 Vitest 文件、76 项；8 项扫描器自测 |
| 独立真实栈 Phase 02–04 Playwright | 通过，25/25（最终一轮约 1.2m） |

覆盖假/驳回/旧版本 review、重复建案、跨作用域、虚假成员拒绝；部分 Draft 允许、提交/核定不足拒绝、确认/份额篡改拒绝；无授权/受益人/提案人/COI 回避，撤销/离队/重加 epoch；未确认修订不影响正式份额、争议排除、推翻历史保留；所有归档阻塞类型、伪造/错误对象/摘要/撤销会话送达拒绝及完整条件归档成功。

真实多连接测试通过 pg_stat_activity 观察第二连接实际处于 Lock 等待后，才释放第一事务，未使用 Mock 模拟竞争：

| 竞争 | 断言 |
| --- | --- |
| 重复建案 | 两请求同案例，仅一个初始版本 |
| 并发调整 | 一次成功，旧 expected_version 拒绝，无丢失更新 |
| 双重确认 | 只有一个不可变确认 |
| 确认 vs 异议（双顺序） | 成功者决定后续状态，另一请求冲突，不泄漏争议统计 |
| 异议 vs 裁定 | 串行化并检测 stale revision，无矛盾裁定 |
| 确认 vs 撤销授权（双顺序） | 权限在 Team 锁后重新核对 |
| 确认 vs 成员退出（双顺序） | 先退出则拒绝核定；先核定则历史保留 |
| 修订/修订确认 vs 归档 | 待修订阻塞；旧版本送达不能满足新核定；先归档则禁止后写 |

另验证 REPEATABLE READ 的旧资格快照及归档被明确拒绝，授权 NULL expected_version 也不能绕过版本检查。故障注入在份额调整、替代确认、推翻裁定的审计写入阶段抛错，逐表摘要证明业务数据、调整、指针、事件和审计全部回滚，无部分写入。测试不接受死锁作为预期失败；遇到 SQLSTATE 40P01 立即失败，最终各并发用例通过。

本地原始日志保留在被忽略的 `.cache/phase05-m2-{integration,database-final,authorization,verify,e2e}.log`，不是远端 CI 结果。

复现命令使用现有测试脚本启动专用库，然后在本地从 `.cache/devtest-pg-password` 读取凭据设置 TEST_DATABASE_URL，执行 `NEXORA_ENV=test NEXORA_TEST_DB_DISPOSABLE=1 go -C apps/api test -race -count=1 -tags=integration ./...`。密码不可打印/提交。验证日志保留在忽略的 `.cache/phase05-m2-{integration,authorization,verify,e2e}.log`；测试库在交付前仅按专用标签清理。

## 10. 回归影响、风险与下一阶段

M2 的最小运行时兼容调整仅有：现有 API 启动允许 schema v6/v7（不自动迁移），以及项目归档约束映射为既有 409；未新增贡献路由、请求/响应契约或 Vue 页面。旧迁移测试固定其历史目标 v6；旧 v6 破坏性 down 测试先撤销空 v7，再验证 v6 仍拒绝删除 Phase 04 证据。

归档新增门禁是批准的行为变化：v7 下有正式验收成果但尚无有效贡献/可信送达时，现有归档路径将拒绝；不能用放弃草案绕过。历史已归档项目保持读取兼容，Phase 02–04 的用户、成员、进度、审核和附件模型未改。

尚待 M3：认证并绑定真实 actor 的贡献 Service/接口、完整业务授权与作用域过滤、输入/证据校验、预期版本冲突处理、人工核定操作流程、结果展示及本人收件确认入口、离队成员受控代提流程。前端、完整通知、综合统计、奖金与发布均未授权实施。

已知限制：本数据库不认证普通 actor_id 参数；不证明任意附件/第三方引用的事实真伪；不能自动发现未申报利益冲突；不提供邮件/短信送达；没有 RLS。现有会话撤销不删除已核验的历史送达，session 外键也会阻止物理清除已引用会话；后续会话保留策略必须保护凭据历史。拥有数据库 owner/DDL 权限的操作者可绕过触发器，应用部署须另行采用受限角色。Team 级串行化沿用既有锁设计，吞吐优化不得改变资格与归档一致性。

M2 必需数据库实现和验证已完成，无未完成的 M2 阻塞项。M2 尚无贡献 API，因此真实业务中缺少核算/收件凭据的归档会保持阻塞，等待 M3 受控流程；当前不会制造送达事实来解除阻塞。

不存在通过修改旧迁移、自动补造成员、后台制造送达或平台角色兜底来“完成”M2 的旁路。提交前仅清理本次创建的专用临时测试容器及其临时凭据，不操作当前开发容器或卷。最终分支保持 `feature/phase05-contributions`，等待验收及下一阶段授权。
