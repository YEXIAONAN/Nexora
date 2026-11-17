# Phase 04 开发与验收报告

日期：2026-11-18。范围：V1.0 方案 B 的真实成果与正式验收。本报告区分功能分支开发、本地验证、远端 CI / 评审和合并；**本阶段不自动合并，不表示 V1.0 已发布**。

## A. Phase 04 功能完成情况

已实现并本地验证：叶子任务成果说明、HTTP(S) 外部/Commit/PR 引用、真实文件、不可变 V1/V2、正式通过/驳回、补正与重新提交、历史详情、明确审核授权和待我审核。仅有分支开发与验证事实，合并事实以 PR 状态为准。

设计先行提交 0638473；数据库 6c3c2ba；事务工作流 1cf5113；私有附件 1f2cd50；真实前端 dd276cf。设计见 [phase-04-design.md](phase-04-design.md)。

## B. 数据库设计与迁移

新增 00006，00001–00005 未修改。新增 task_submissions、task_submission_versions、task_reviews、project_reviewers、submission_attachments，以及当前版本/批准指针、复合外键、授权 epoch 和 task_acceptance_evidence 视图。

版本与审核禁止 UPDATE / DELETE；已绑定附件禁止变更；唯一约束防止版本号或决定重复。Team 首锁与任务锁、expected_version、延迟证据约束共同保护状态和版本。SQL 直接修改 completed、假 review UUID 或缺失批准被真实 PostgreSQL 拒绝。

`TestPhase04FreshUpgradeAndRollback` 验证 v5→v6、空库安装、空业务 down 到 v0 后重建；`TestPhase04RollbackRefusesBusinessEvidence` 验证存在成果时 down 拒绝并保留 v6。已有 Phase 03 v5 专项测试显式固定升级目标 5；未改历史迁移。没有升级正在运行的旧库。

## C. 成果版本模型

任务唯一成果根，版本 UUID 与单调整数并存，记录作者、内容、引用、附件、提交时间、任务版本、request_id 和请求摘要。相同作者/幂等 UUID/归一化内容重放返回原版本；不同内容冲突。并发相同提交只创建一个版本。V1 驳回不覆盖，V2 独立审核，旧版本不能触发当前完成。

证据：`TestImmutableVersionReviewProgressDependency`、`TestSubmissionReplayAndConcurrentReviews`、`TestRejectedDirectResubmissionAndCancelledDependencies`；浏览器五个 Phase 04 场景读取真实版本号、旧意见、当前批准指针。

## D. 审核权限与状态机

默认项目负责人及显式授权的有效项目成员可审核，任何身份均不能自审。ROOT / Team owner/admin / 任务参与均不产生隐式审核权。Team / Project 移除立即失效；重新加入不恢复旧授权。授权、撤销与原因追加审计。

in_progress/rejected 通过新成果进入 awaiting_review；当前版本 approve/reject 原子进入 completed/rejected；rejected 带原因进入 in_progress 或取消。普通接口仍不能直接进入验收或完成。完成、取消终态，待验收冻结编辑/权重/结构；归档只读。

待验收期间原负责人离队也不会卡住独立审核：只允许正式审核改变状态和批准指针，其他字段冻结；延迟约束继续要求当前有效审核证据。该边界已先复现 approve/reject 均失败，再修复并回归（TestFormalReviewSurvivesHistoricalOwnerDeparture）。

证据：授权、自审、撤销、成员 epoch、归档测试；`TestDefaultLeadCanReviewOtherAuthor`；两个并发相反审核仅一条合法决定。HTTP 和 E2E 再次拒绝自审与旧版本审核。

## E. 进度、依赖、历史、审计联动

保留整数精确权重：pending 0、in_progress 最多 79%、awaiting_review 80%、rejected 0、取消剔除分母。completed 只有当前版本实际通过才 100%。旧 actual_progress 不被驳回删除；驳回、补正、提交与审核均保留历史和快照。父任务只汇总有效叶子。

依赖每次从 scoped 数据库证据重算；当前批准后解除，取消/驳回/待验收不解除。`TestDatabaseRejectsForgedEvidenceAndRollback` 注入审核审计失败，确认任务版本、审核、快照、任务批准审计全部回滚；`TestCompletionRequiresCurrentVersionApproval` 验证无/错版本/拒绝/假指针均不产生 100%。

## F. 附件安全与存储

单实例私有 os.Root 目录；0700/0600，服务端 UUID 键、O_EXCL、fsync，路径与符号链接逃逸被拒绝。验证扩展名、内容嗅探、UTF-8/图片配置，拒绝 HTML/SVG/可执行/空/超限；支持 txt/md/csv、PDF、PNG/JPEG。10 MiB/文件、5 文件/25 MiB 每版本、512 MiB 每 Team。下载再次授权，校验大小及 SHA-256，attachment/octet-stream/nosniff/private no-store/sandbox。

暂存 24h，提交同事务绑定，已绑定永不自动回收。失败写入回滚清理；不确定提交结果先用新连接查证，未知则留给宽限回收。过期文件物理删除成功后才标 removed_at 并释放配额；缺失已绑定文件返回 503，保留证据。没有复用其他项目存储，没有声称病毒扫描或多副本能力。

证据：store 单元测试；三个 attachments 集成测试覆盖真实上传/下载、幂等、跨任务拒绝、不可变元数据、配额、限额、缺失文件、TTL/孤立回收与审计失败清理；HTTP 测试原始文件名穿越、CSRF、私有头和匿名拒绝。数据库/文件备份边界已写入 README 和运维文档。

## G. API 及前端功能

OpenAPI 增加 12 操作，总计 61，生成类型一致。真实 API 接通提交、版本、审核、历史、上传/下载、授权和 inbox。Vue 显示作者/版本/审核人/结论/意见、当前状态、实际项目进度；明确“已提交尚未完成”。

前端所有写入捕获已确认 Team/Project/Task/Version 与任务并发版本；组件 generation 阻止旧响应覆盖、旧上传链继续提交。Vitest 覆盖切换 Team / Project / Task、旧审核错误、上传中切任务和历史版本禁止审核。桌面、深色和手机截图见 [真实浏览器证据](evidence/phase04/README.md)，没有提交密码、Cookie 或 trace。

## H. 单元、集成、E2E 与 CI

本地实际通过：

| 门禁 | 实际结果 |
| --- | --- |
| make verify | gofmt/vet、Go 单元、ESLint/Prettier/TS、构建、API 类型、密钥扫描通过 |
| PostgreSQL 17 integration + race | 全包通过，独立 nexora_test、随机测试 schema、testdb 护栏未放宽 |
| 迁移 | v5 升级、空库安装/回滚重建、有业务证据拒绝回滚均通过 |
| Vitest | 15 文件、76/76 |
| Chrome Playwright | Phase 02 11 + Phase 03 9 + Phase 04 5，25/25；真实 UI/HTTP，未设置测试后门 |
| Docker | Compose 静态校验及 API/Web 两个镜像构建通过；未启动或重建旧栈 |

可复现命令见 [README](../../README.md) 和 [testing.md](testing.md)。本地完整日志保存在被忽略的 `.cache/phase04-{integration,e2e,docker-api,docker-web}.log`；验证曾发现并修正前端重复声明、原始 select 可访问名称、E2E 保存等待及 IPv6 host 差异。Web 镜像源曾 TLS 超时，重试构建成功，不把失败尝试计为通过。

远端 CI 与审查属于独立门禁，本报告提交时尚未合并；最新 HEAD 的 Actions / Review 链接和状态记录于 [Phase 04 PR](https://github.com/YEXIAONAN/Nexora/pull/4)，不能用旧 develop CI 代替。

## I. README 修改对比及安全纠错

从 Phase 03 PR 待合并/Phase 04 未开发的旧说明，改为 Phase 01–03 已合并、本次 Phase 04 分支待合并；标题 Nexora，明确已批准 V1.0 方案 B、V1.1 贡献、V2.0 长期范围。增加功能矩阵、ROOT→Team→项目任务→版本验收流程、私有存储与部署限制、文档入口和 main 后续发布清单。

安装/启动/迁移/构建命令逐项对照真实 Makefile、package.json 与脚本；corepack prepare 已核对可用。集成示例使用 test-db.sh 的独立 loopback 5433 / nexora_test，不推荐普通 nexora 库。test-db.sh start 已执行，脚本仅操作带专用 disposable 标签的同名容器；make verify / e2e 和容器构建实测。默认启动自动迁移改为 false；明确旧运行栈不一定已升级。备份脚本仍只覆盖数据库，新增同恢复点文件备份要求。

## J. Git 分支、提交、PR、Review 状态

从最新 origin/develop c360327eef5ca73abb5c99379afc346815541a37 建立 feature/phase04-submissions-reviews。开工核对 develop CI 44118052169 四项成功。阶段 Conventional Commits 如 A；本报告随 M6 文档提交，已创建 [PR #4](https://github.com/YEXIAONAN/Nexora/pull/4)（此分支→develop）；最新 HEAD 的 CI 运行链接和必要评审/整改证据在 PR 中更新。发现阻断需修复/回归/重评。

不直接提交 develop / main，不自动合并。Phase 03 报告保留其当时的测试与 PR 事实，本次通过新报告记录其已合并。最终 PR / Review 状态以该 PR 的实时元数据为准；未宣称已合并。

## K. Phase 05 贡献核算准备情况

可承接证据包括：任务/Team/项目、当前成果版本、不可变作者与提交时间、正式批准 review_id / reviewer / reviewed_at / request_id、版本内引用与附件摘要、权重和追加式进度/审计。Phase 05 应只读取 task_acceptance_evidence 所对应的当前已批准证据，并以唯一验收 ID 幂等生成贡献；贡献规则、比例、人工核定和奖金均未在本阶段开发或批准。

## 强制场景覆盖映射

| 请求编号 | 自动化证据 |
| --- | --- |
| 1–3 | SubmitAndReviewAuthorization、DefaultLeadCanReviewOtherAuthor、附件上下文与既有 Team 隔离测试 |
| 4–5 | SubmissionStatesAndArchive、Phase 03 结构/归档回归 |
| 6–9 | ImmutableVersionReviewProgressDependency、RejectedDirectResubmissionAndCancelledDependencies、Phase04 E2E |
| 10–11 | DatabaseRejectsForgedEvidenceAndRollback、普通 transition 单元/HTTP 回归 |
| 12–14 | SubmitAndReviewAuthorization、ReviewGrantDoesNotResurrectAfterRejoin |
| 15–16 | SubmissionReplayAndConcurrentReviews（真实并发 PostgreSQL） |
| 17–21 | CompletionRequiresCurrentVersionApproval、ImmutableVersionReviewProgressDependency、取消依赖测试和 E2E |
| 22 | 审核审计故障注入整体回滚 |
| 23–25 | FileValidationAndRootedStorage、PrivateAttachmentBoundToImmutableVersion、AttachmentScopeLimitsFailureAndRecovery、AttachmentQuotaAndVersionTotal、HTTP 私有下载 |
| 26 | SubmissionPanel / ReviewInbox / ProjectDetailView 的可控 Promise 竞态测试 |
| 27 | README 对照 Makefile、现有脚本及实际 gate 执行，corepack 帮助核查 |
| 28 | 全包 integration/race、76 Vitest 和 25 E2E 包含全部 Phase 02/03 用例 |
