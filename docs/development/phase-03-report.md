# Nexora V1.0 Phase 03 开发与验收报告

日期：2026-11-17（Asia/Shanghai）。状态：功能已在 feature/phase03-projects-tasks 开发并本地验证；PR、CI 和 Review 交付状态见 G。尚未合入 develop，不标注为正式集成完成，更不表示 V1.0 已全部完成。

## A. 功能完成情况

| 模块 | 交付内容 | 代码 |
| --- | --- | --- |
| 项目 | 当前 Team 列表/详情、创建/编辑、负责人指定与受控交接、成员增减、里程碑计划、归档只读 | internal/projects |
| 三级任务 | 创建/详情/编辑、父子同项目、整棵子树移动、深度上限、负责人/多协作者、优先级和计划日期 | internal/tasks |
| 状态机 | pending→in_progress；pending/in_progress→cancelled（原因必填）；非法转换 409；父节点不走叶子动作 | tasks.Service + 数据库执行触发器 |
| 依赖 | 同项目非取消叶子、自环/重复/全图环检测、阻塞、逐依赖显式忽略与审计 | tasks/dependencies.go |
| 进度 | actual/effective 分离、79% 上限、仅叶子权重、取消排除、精确计算、权重事件与不可覆盖快照 | tasks/domain.go、history.go、repository_history.go |
| 权限与审计 | Service 实时 Team/项目授权、复合 FK、成员退出/交接保护、同事务业务/历史/审计、expected_version 冲突 | projects.Within、teams、audit |
| Vue 前端 | 项目列表/创建、详情工作区、成员、任务树/编辑/分配/执行/依赖、里程碑、历史、设置/交接/归档 | ProjectsView、ProjectDetailView、components/projects |

M1 先冻结设计/权限/状态/算法/OpenAPI，随后按 M2–M5 实现，M6 进行全量门禁与 PR 交付。Phase 04 成果、版本、审核、驳回、重提、正式完成全部未开发，未创建假的 submission/review 表或记录。

## B. 数据库

仅新增迁移 00004_projects.sql 和 00005_tasks.sql，源码 schema v3→v5；00001–00003 未修改。完整字段/约束/索引表见 [数据库文档](../architecture/database.md#phase-03-schema特性分支开发与验证)。

新增八表：projects、project_members、project_milestones、tasks、task_collaborators、task_dependencies、task_progress_history、project_progress_snapshots。

- 业务 UUID、历史 bigint；全部业务与历史表带 team_id。项目/任务/人员复合外键防跨 Team 和跨项目挂接，重要历史默认 RESTRICT，无级联删除。
- tasks：六状态枚举、depth 1–3、actual 0–99、weight 1–1000000、计划先后与取消原因一致性；项目 active/archived 与 archived_at 一致。
- projects.lead_id 是唯一负责人来源，project_members 由此推导 lead/participant，不重复维护可矛盾的角色列。成员软移除保留历史。
- Team/项目成员退出触发器保护负责人交接和非取消任务分配；任务执行触发器验证 active owner 并拒绝 Phase 04 状态；延迟结构触发器检查最终父子 depth 差值，严格递增的深度也排除环。普通 CHECK 不声称能验证跨表 active 或递归关系。
- 历史与快照复用 audit 追加式触发器，拒绝 UPDATE/DELETE。快照精确 numeric 仅允许非负整数字符串，拒绝 NaN/Infinity/小数；业务计算不使用浮点。
- 索引覆盖项目列表/负责人、项目成员资格、里程碑计划、任务树/负责人/协作者、依赖反向引用、任务历史与项目快照倒序查询。

真实迁移测试 TestPhase03FreshUpgradeAndRollback：空库 up→5；down 两步→3；v3 up→5；完整 down→0；重新 up→5，均通过。另有迁移幂等与随机 schema 隔离验证。一次性容器 nexora-devtest-pg（127.0.0.1:5433，PG17）与显式 nexora_test 库承载集成测试；E2E 使用同一一次性实例的专用 nexora_e2e 库，退出删除。

运行开发服务只读 /readyz 观察仍为 version 1，属于旧运行栈；从未在其上迁移、升级或重启。未接触其他项目数据库/卷，未使用 prune、compose down -v 或破坏性 Git 命令。

## C. 业务规则

1. **三级树**：Service 在 Team 锁内遍历父链和整棵子树，拒绝自后代、跨项目/Team、四级深度；移动同时更新所有后代 depth/version。非叶集合缓存随结构变动失效，避免反复扫描判定叶子。
2. **父/叶区别**：父节点显示状态与有效进度由后代叶子派生，actual 不可填报，自身权重不计。叶转父须 pending、零 actual、无执行事件且无任何入/出依赖（含已忽略）。没有清除历史或删除依赖来满足转换条件。取消只作用于叶子，无隐式子树批量取消。
3. **状态/证据**：只允许开始与取消。手填 100 返回 422；没有成果版本和正式验收不能进入 awaiting_review/completed/rejected（409，数据库另有防线）。Phase 03 不把 completed 字符串当作验收证据，纯函数对该无证据状态保守返回 0。
4. **依赖**：A→B 表示 A 依赖 B。增加限 pending 叶子，避免给进行中任务追加未满足前置。DFS 包含已忽略边防环；无合法验收证据时未忽略的依赖阻止开始与修改进度，B 取消仍阻塞 A。忽略保留原行、actor/原因/时间并写审计，不改变 B 状态。
5. **算法**：叶子 pending=0，in_progress=min(actual,79)×100 bps；未来 awaiting_review 默认 8000（当前不可写）。取消叶子完全排除。以 math/big 累加 weight×bps 和 weight，整除向下取整至 0.01%。空分母 0，父节点聚合同样规则。前端只格式化服务端 bps，不另算业务进度。
6. **历史**：创建、移动、开始/取消、实际进度、权重修改均同事务写 task_progress_history + project_progress_snapshots + audit。权重记录 old/new、原因、actor、生效时刻、修改前后项目 bps；旧快照不覆盖、不因新权重重算。时间使用锁后 clock_timestamp，最终可见性在提交时生效。
7. **授权与并发**：统一 Team 行锁与 Phase 02 成员变更串行，锁后重查资格；业务读也在该锁内取得一致的多查询视图。expected_version 防止旧页面丢失更新。平台角色不绕过 Team，Team owner/admin 仅额外创建和显式交接，不隐式获得项目管理。任务负责人/协作者离队后历史保留、执行失效；需重新分配或明确重新授权。

ADR 0005 只接受进度部分；金额、舍入及余额归属仍 Proposed，原提议与决策演进保留。

## D. API

通用前缀 `/api/v1/teams/{teamID}/projects`，以下 `{P}` 代表该前缀加 `/{projectID}`，`{T}` 代表 `{P}/tasks/{taskID}`。新增 25 个 operation；全部使用会话，写操作要求 CSRF，资源归属由 URL/数据库决定，请求体不能覆盖。

| 方法 | 路径 | 权限 |
| --- | --- | --- |
| GET | 项目前缀 | active Team 成员 |
| POST | 项目前缀 | Team owner/admin，指定 lead 必须 active |
| GET | {P} | active Team 成员 |
| PATCH | {P} | 项目负责人 + expected_version |
| POST | {P}/archive | 项目负责人 + expected_version |
| POST | {P}/handoff | 项目负责人或 Team owner/admin；原因、active 新 lead、版本 |
| GET | {P}/members | active Team 成员 |
| POST | {P}/members | 项目负责人；active Team 成员 |
| DELETE | {P}/members/{userID} | 项目负责人；非 lead、先移交非取消任务分配 |
| GET | {P}/milestones | active Team 成员 |
| POST | {P}/milestones | 项目负责人 |
| PATCH | {P}/milestones/{milestoneID} | 项目负责人 + 里程碑版本 |
| GET | {P}/tasks | lead 全树，参与者仅负责/协作节点；服务端项目进度 |
| POST | {P}/tasks | 项目负责人；父子/人员/权重合法 |
| GET | {T} | lead 或该任务的有效负责人/协作者 |
| PUT | {T} | 项目负责人 + 任务版本 |
| POST | {T}/move | 项目负责人 + 任务版本，整棵子树校验 |
| POST | {T}/transition | 开始：lead/负责人/协作者；取消：lead/任务负责人；版本 |
| PUT | {T}/progress | lead/任务负责人/协作者；未阻塞 in_progress 叶子、版本 |
| PUT | {T}/weight | 项目负责人；正整数、原因、版本 |
| POST | {T}/dependencies | 项目负责人；合法前置叶子、版本 |
| POST | {T}/dependencies/{dependencyID}/ignore | 项目负责人；具体依赖、原因、版本 |
| GET | {T}/history | 同任务读取权限；最近 200 条 |
| GET | {P}/progress | active Team 成员 |
| GET | {P}/progress/history | active Team 成员；最近 200 条 |

归档拒绝所有上述写入。无 Team 或跨 Team/Project 404；有成员关系但角色不足 403；状态/旧版本冲突 409；输入非法 400/422。统一 data/error 信封及 request_id。OpenAPI 生成类型已校验一致。

## E. 安全和测试

真实通过的门禁：make verify（gofmt、vet、Go 单元、ESLint、Prettier、vue-tsc、Vitest、Go/Web 构建、契约一致、密钥扫描）；go vet -tags=integration；真实 PG17 的 go test -race -count=1 -tags=integration ./...；Compose 完整配置；API/Web Docker 镜像构建；浏览器 E2E。

| 强制矩阵 | 证据 |
| --- | --- |
| 跨 Team/Project 查询/修改、非有效 lead、参与者不能管理、离队失权 | projects 的 IsolationPermissionsAndQualification；tasks 的 IsolationAssignmentsAndParticipantAccess；HTTP 工作流与 Phase 02 全量回归 |
| 交接、归档只读、旧版本 | HandoffArchiveMilestonesAndVersions、HTTPRealWorkflow、E2E |
| 一至三级、第四级、自后代、子树深度、执行后叶转父、父进度/状态拒绝 | TaskHierarchyAndSafeConversion、ConcurrentSubtreeMovesPreserveDepth |
| 开始、取消原因、终态、完成拒绝、100% 拒绝、并发旧版本 | TaskStateMachineAndOptimisticConcurrency、Phase03TransitionsOnly（全六状态组合）、HTTPRealWorkflow |
| 同项目依赖、自环/重复/环、跨项目/Team、阻塞、取消前置、显式忽略审计 | DependenciesBlockingIgnoreAndCycles、DependencyIsolationAndConcurrentCycle、DependenciesRejectCrossProjectAndCrossTeamReferences、IgnoredDependencyStillParticipatesInCycleDetection、ConcurrentDependenciesCannotFormCycle |
| 权重、仅叶、取消、空、79% 上限、无证据不计完成、精确整除、大数/非法值 | AggregateExactProgress、OnlyLeavesAndAllDescendantsCount、WeightHistorySnapshotsAndArchiveProtection |
| 历史不被覆盖、审计失败事务回滚 | 权重快照比较、ProjectAuditFailureRollsBack、TaskAuditFailureRollsBackStateAndHistory |
| 成员移除与执行并发、重新入队不自动恢复项目资格 | ConcurrentRemovalAndExecutionRespectMembership、TeamRejoinRequiresExplicitProjectReauthorization |
| 新库/v3 升级/回滚与数据隔离 | Phase03FreshUpgradeAndRollback、MigrationsApplyIdempotently、testdb 护栏回归 |
| 前端切换与迟到操作 | ProjectDetailView 5 项可控 Promise 测试，含 A→B→A；原 TeamMembersView 9 项回归 |

没有批量业务接口，因此批量越权整批回滚专项不适用；混合无资格任务人员在落库前整体拒绝（InvalidCollaboratorSetLeavesAssignmentUnchanged）。未进行生产部署、TLS 终止、压力测试、全面渗透测试；不声称这些已验证。没有 Phase 04 业务证据，自然也未测试真实验收通过/驳回/重提。

## F. 前端与 E2E

Vue 3 延续现有组件/色彩/响应式样式，没有新增大型 UI 库。数据全部来自真实 HTTP API，无 Mock 伪装。加载、空列表、无权限/失效资格、错误和旧版本均有页面反馈。项目上下文变化立即清除已确认状态，旧响应不得落地；所有写入捕获已确认 Team/Project，迟到成功或失败不影响新上下文（含 A→B→A）。

make e2e 在一次性真实栈运行 **18/18 通过**：Phase 02 原 11 项 + Phase 03 新 7 项。新场景：

1. 登录→选择 Team→UI 创建项目。
2. 真实受控账号/邀请加入→UI 添加项目参与者。
3. UI 创建一、二、三级任务→负责人分配→协作者编辑；API 断言层级与仅叶权重。
4. 普通参与者仅见承担任务，开始并记录实际 60%，项目显示 60.00%。
5. 实际 99% 时有效 79%；新叶权重 1 后项目 59.25%；配置依赖阻塞；原因明确忽略后才开始。
6. 权重变更后项目 39.50%，历史仍可见 59.25%；里程碑、项目编辑、归档后只读且后端拒绝写入。
7. 顶栏切换 Team，离开旧项目上下文，旧项目名称不在新 Team 页面显示。

已检查桌面浅色、桌面深色、390px 手机截图，手机无横向溢出。截图/测试日志保存在本地忽略目录 .cache；不把随机口令、环境文件或临时运行数据提交到仓库。

## G. GitHub

- 基于重新 fetch 的 develop c3fb029（与指定基线相同），合并 CI 44097502361 四项 success；Phase 02 的两个审查线程 resolved，开放 Issue 查询为空。
- 分支 feature/phase03-projects-tasks，M1–M6 使用普通 Conventional Commits，无历史重写、force push 或 AI 联合署名。
- 本地交付提交与 PR/CI/Review 链接在创建 PR 后补充；当前不得仅因本地通过就声称全部最终条件已满足。
- main 36bddd6 与 develop c3fb029 不由本次任务修改。禁止自动合并。

## H. Phase 04 准备与未开发边界

可承接：稳定 Team/Project/Task UUID、叶子判定、实际/有效进度分离、版本冲突控制、统一授权事务、追加式历史/快照及审计。当前六状态枚举预留未来状态；Phase 04 必须先追加提交/版本/正式审核的数据结构和复合外键，再用新增迁移替换“拒绝未来状态”的执行保护，并把完成/解除依赖的判断绑定当前成果版本的有效正式验收记录。

**尚未开发**：成果提交与版本、审核通过/驳回、补正与重新提交、正式完成、贡献证据/核算。当前没有这些接口或伪造证据，也不能手工用状态或 100% 进度解除依赖。

已明确的首版限制：同 Team 业务读写串行，适合小团队、未做大规模吞吐验证；历史 API 返回最近 200 条但数据库完整保留；没有父任务依赖/批量取消/物理删除；里程碑仅为计划。归档严格只读，连负责人交接也拒绝，因此归档前须安排长期保管负责人，归档负责人离队被保护拒绝；未来若需更正应单独设计受控修订。活跃项目负责人可先交接再退出。

验收过程修正：新增迁移的一条重复里程碑索引定义已删除，并重跑空库/v3 升级/回滚与全量集成；原 Phase 02 会话失效 E2E 增加等待初始加载完成的条件，避免页面已正确跳到登录后测试仍尝试点击旧刷新按钮。未增加随机 sleep 或以自动重试隐藏失败。
