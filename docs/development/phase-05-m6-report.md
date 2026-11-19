# Phase 05 M6 综合验收与发布准备报告

日期：2026-11-19（Asia/Shanghai）。分支：`feature/phase05-m6-acceptance`。基线：develop `9e35a288c41aa0b5b71753b3e02ca738647cd299`，M5 PR #8 Merge Commit；合并后 develop CI [44505800232](https://github.com/YEXIAONAN/Nexora/actions/runs/44505800232) 四项成功。Schema 保持 v7，未修改任何历史迁移。

## 1. 结论摘要

| 状态 | 结论 |
| --- | --- |
| M6 功能验收 | **PASS**：完整业务闭环、安全/权限、告知幂等、联合恢复、回归门禁均通过 |
| V1.1 候选版本发布准备 | **有条件**：M6 评审尚未合并；生产规模性能/分页、离队成员可信送达、生产部署未验收 |
| 生产部署实际验收 | **Not Tested**：本轮只在隔离开发环境执行 |
| main 正式发布 | **未发生**：未修改 main、未自动合并、未发布 |

未发现需要修改生产代码的 P0/P1 缺陷。M6 新增的是综合验收测试、联合恢复演练脚本和文档；生产 Go/Vue/OpenAPI 代码与 v7 迁移未改动。PR #9 评审提出的三项整改（恢复脚本安全门禁、跨平台附件校验、真实跨 Team 夹具）已按下文 §12 修复并重新验证。

## 2. 完整业务闭环 E2E

真实 PostgreSQL 17.11、Go API、Vue 页面、Chrome 独立会话；没有 route mock、数据库伪造或直接插入业务表。全量 Playwright **31/31 通过**（1.4m），覆盖：

| 业务流程 | 证据 |
| --- | --- |
| ROOT 登录、创建 Team、管理员创建账号、邀请/接受 | `phase02.spec.ts` 1–11 |
| Team 切换与项目/任务数据隔离 | `phase03.spec.ts` 12–20 |
| 项目、成员、三级任务、执行与进度 | `phase03.spec.ts` 12–19 |
| 正式成果 V1/V2、真实附件、独立审核 | `phase04.spec.ts` 21–25 |
| 真实 review_id 建贡献案例、人工 10000 基点草案 | `phase05.spec.ts` 26–27 |
| 独立核定、正式版本不可变 | `phase05.spec.ts` 28 |
| 准备告知、本人结果、显式 POST 收件（GET/刷新 0 次收件） | `phase05.spec.ts` 28 |
| 本人异议、独立裁定、版本修订与重新核定 | `phase05.spec.ts` 29–30 |
| 新版本重新告知/收件，旧收件不能用于新版本归档 | `phase05.spec.ts` 30 + M5 服务测试 |
| 满足全部约束后项目归档成功 | `phase05.spec.ts` 30（M6 新增：V3 告知、本人收件、归档 200） |
| 离队成员普通 case/results/project 404，仅隐私 intake | `phase05.spec.ts` 31 |

归档验证使用真实 V3 有效确认和真实收件证据；未降低归档约束，未用离队成员例外绕过。

## 3. 安全与权限

完整 Go 集成套件在真实 PostgreSQL 17.11、`-race` 下全部通过。关键矩阵：

| 场景 | 结果 | 自动化证据 |
| --- | --- | --- |
| 跨 Team/Project IDOR | PASS | `TestHTTPContributionRejectedReviewAndCrossTeamIDOR`、`TestURLTeamIDTamperingIsNotFound`；M6 `m6_outsider` 只加入第二个 Team，源/恢复环境 attachment/case/results 均 404 |
| 无效/撤销/过期 Session | PASS | `TestHTTPContributionSessionFailures`、`TestProtectedEndpointRequiresSession` |
| CSRF 缺失 | PASS | `TestTeamRoutesRequireSessionAndCSRF`、`TestLoginRequiresCSRFHeader` |
| 自我核定、受益人/提案人/来源审核人回避 | PASS | `TestContributionBeneficiaryProposerAndDeclaredConflict`、`TestServiceContributionIndependentProposerAndConflicts` |
| 已撤销核定授权、成员 epoch | PASS | `TestContributionAppraiserIndependenceAndEpoch`、`TestContributionRejectsStaleAuthorizationSnapshots` |
| 离队与重新加入不恢复旧权限 | PASS | `TestServiceContributionAuthorizationEpoch`、`TestReviewGrantDoesNotResurrectAfterRejoin`、`TestContributionConcurrentRevocationAndMemberExit` |
| 未确认草案冒充正式贡献 | PASS | `TestHTTPContributionLifecycle`、M4/M6 E2E 的 effective=null 断言 |
| 旧/伪造 review_id 建案 | PASS | `TestContributionSourceScopeAndMembership`、`TestDatabaseRejectsForgedEvidenceAndRollback` |
| 直接 SQL 修改不可变历史 | PASS | `TestContributionDatabaseLifecycle`、`TestContributionApplicationRoleCannotBypassHistory` |
| 伪造收件人/摘要/核定版本 | PASS | `TestContributionDeliveryCannotBeForged`、`TestContributionArchiveAndVerifiedDelivery` |
| ROOT 跨 Team 访问 | PASS | `TestHTTPContributionIdentityAuthorizationAndValidation`、`TestAdminTeamsRequiresPlatformRole` |
| 附件路径穿越、符号链接、文件权限 | PASS | `TestFileValidationAndRootedStorage`（`../`、反斜杠、控制字符、symlink、0600、篡改拒绝） |
| 私有附件下载/越权 | PASS | `TestPrivateAttachmentBoundToImmutableVersion`、`TestSubmissionHTTPAndPrivateDownloadHeaders` |
| 受限应用数据库角色真实业务 | PASS | `TestContributionApplicationRoleCannotBypassHistory`（独立 LOGIN、文档 GRANT 与真实操作） |
| 关键错误日志泄露凭据/会话/私有证据 | PASS | `TestDiagnosticLogsDoNotExposeDatabaseErrorDetails`、`TestSafeErrorPreservesClassificationWithoutCredentials`、`TestServiceContributionAuditFailureIsAtomicAndSanitized` |

未发现新的 P0/P1 权限缺陷。

## 4. 告知幂等与历史一致性

M5 业务键保持：`team + project + task + case + case_version + confirmation + recipient + authenticated_ack`。

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| 相同 client_request_id 重放 | PASS | `TestServiceContributionNoticeIntentDeduplication` |
| 不同请求 ID、同一业务意图 | PASS | 同上；返回最早告知，只保留一行 |
| 并发相同意图 | PASS | `TestServiceConcurrentNoticeIntentIsSingle`（真实 Team 锁等待） |
| 响应丢失后新 key + 旧 revision 重试 | PASS | `phase05.spec.ts` 28 的新增真实 HTTP 重试 |
| 新修订版本重新告知 | PASS | 新 confirmation 独立告知；旧收件不能满足新版本归档 |
| 既有重复历史不删除 | PASS | 测试直接插入 legacy duplicate 后仍保留，新请求不新增第三条 |
| 收件绑定正确 confirmation_id 与 digest | PASS | 收件测试 + 恢复后结果/历史核对 |
| SQL 旁路写入风险 | **风险已知** | 没有数据库唯一约束；绕过 Service 直接 INSERT 仍可制造重复，必须通过代码审查/DB 权限约束 |

**旧确认被 superseded 后的语义（事实与建议）**：对旧 `case_version_id + confirmation_id` 使用新请求 ID 时，服务按幂等返回历史告知，不创建新告知、不产生新收件证据；归档仍只检查当前有效版本。这是 M5 已实现的“同业务意图返回最早记录”语义，M6 用 `TestServiceContributionNoticeHistoricalIntentIsNotNewDelivery` 固定。风险是接口仍返回 201，调用方可能误以为这是新版本告知；建议在后续契约中增加 `reused_existing` 标识或对非当前有效确认返回 409。该项未经新产品政策批准，本轮未改变行为。

## 5. 数据库与附件联合备份恢复

演练脚本：[infra/scripts/m6-split-restore.sh](file:///Users/waiting/Desktop/Project/Nexora/infra/scripts/m6-split-restore.sh)；真实 HTTP 夹具：[m6_flow.py](file:///Users/waiting/Desktop/Project/Nexora/infra/scripts/m6_flow.py)。仅使用一次性 `nexora-devtest-pg`（loopback 5433）和 `nexora_m6_*` 数据库。

环境：macOS 主机，Docker 24.0.6，PostgreSQL 17.11（`postgres:17-alpine`），源/恢复 API 分别监听 18081/18082，私有附件目录在 `.cache/m6-split/<stamp>/`。脚本已移除 macOS 专属 `stat -f`/`shasum` 调用，附件大小、SHA-256 与 0700/0600 权限统一由 Python 3 读取；本轮在 macOS 实测，Linux 实际执行未测试（**Not Tested**），无平台专属命令残留。

演练数据与结果（最终整改后重跑，`20261119T115115Z`）：

- 真实 HTTP 创建 5 用户、2 Team、1 项目、1 任务、1 成果版本、1 审核、1 附件（108 字节）、1 贡献案例/版本/确认/告知/收件、57 条审计事件、5 个会话；`team_members=6`（test Team 4 + 第二 Team 2）。
- `m6_outsider` 只接受第二个 Team 的邀请，明确不在被测资源所属 Team；源环境与恢复环境均通过真实成员列表核对，并验证附件、案例、本人结果 404，第二 Team 成员列表 200、被测 Team 成员列表 404。
- 暂停 source API 后 `pg_dump`（269,610 字节，SHA-256 `25774dbe…a9e8`），复制匹配附件目录；清单 1 个文件、108 字节、SHA-256 `0d7dc95c…a125`。
- 恢复到 `nexora_m6_restore` 和第二个附件目录；schema v7、16 张业务表行数逐表一致。
- 每个 bound 附件的 `storage_key`、大小、SHA-256 与数据库一致；目录 0700、文件 0600。
- 恢复 API 用真实 worker 会话读取本人结果、确认收件、下载附件成功；其他 Team 用户 case/results/attachment 404；匿名附件 401。
- 删除附件后下载 503；写入同大小错误字节后下载 503；恢复原文件后 200。
- 联合恢复总耗时（恢复 + 全量校验 + 200 请求负载 + 缺失/损坏检查）**11.92 秒**。
- 破坏性操作安全门禁：错误容器标签、错误端口、停止容器、缺失凭据四种场景先于任何数据库操作退出，回归测试证明 `DROP DATABASE` 调用次数为 0（见 §12）。

该结果证明当前单文件规模下的恢复流程；未验证大附件量/大数据库/生产存储，不把它冒充生产 RTO。

## 6. 性能、并发与可观测性

环境：同一隔离 PG17.11、恢复 API、单项目/单案例；客户端 Python 8 线程、200 次 `GET /contribution-results/mine`，预热后测量。

| 指标 | 结果 |
| --- | --- |
| P50 / P95 / max | **98.55 ms / 123.15 ms / 132.54 ms**（mean 101.72 ms） |
| 状态 | 200/200 成功，无 401/409/500 |
| 连接池配置 | 默认 `NEXORA_DB_MAX_CONNS=10`、min=0、max lifetime=1h（配置源码核对，未做饱和压测） |
| 我的贡献结果请求数 | 1 项目/1 案例 = 6 个前端请求（listProjects、mine、members、detail、evidence、versions 各 1）；多案例按唯一 case 复用，M6 单测断言调用次数 |
| 代表性查询计划 | 单行夹具下 `contribution_notifications` 与 `submission_attachments` 均 Seq Scan（0.283 ms / 0.085 ms），符合小表规划；索引存在但大规模计划未测试 |
| 高并发核定/告知锁等待 | PASS：`TestServiceConcurrentNoticeIntentIsSingle` 等真实 PG 锁等待测试；无 40P01 死锁、无超时 |
| 日志泄露 | PASS：安全测试覆盖数据库错误详情、Session、Cookie 不收据化输出 |
| 大量项目分页/批量性能 | **Not Tested** |
| 生产规模 P50/P95、RTO/RPO | **Not Tested** |

P50 约 99 ms 是当前本机/夹具观测值，不代表生产 SLO；本轮不据此声明性能达标。

## 7. 测试清单

| 检查 | 结果 |
| --- | --- |
| `make verify` | PASS（Vitest 25 文件/209 项、lint、format、typecheck、build、OpenAPI、secret scan、恢复脚本安全回归 4 项） |
| Go 全量 `-race -count=1 -tags=integration ./...` | PASS（PG17.11；contributions 146.377s、database 110.517s、httpapi 79.298s） |
| Vitest 全量 | PASS 209/209（M5 基线 209，未删减） |
| Playwright 全量真实栈 | PASS 31/31（M5 基线 31，未删减） |
| Secret scan | PASS（含扫描器 8 项自测） |
| 恢复脚本安全回归 | PASS 4/4（错误标签、错误端口、停止容器、缺失凭据，`DROP DATABASE` 均为 0） |
| 容器构建 | 由 PR CI Containers job 执行；本地未重复构建 |
| 数据库 + 附件联合恢复 | PASS（11.92s，见 §5） |
| 并发/权限专项 | PASS（见 §3–§6） |
| Push CI [44519475619](https://github.com/YEXIAONAN/Nexora/actions/runs/44519475619) | PASS，head `0cbe5ee`，8/8 |
| PR CI [44519487025](https://github.com/YEXIAONAN/Nexora/actions/runs/44519487025) | PASS，head `0cbe5ee`，8/8 |

## 8. 缺陷、修复与提交

M6 综合验收未发现需要修改生产代码的阻断缺陷。新增：

- 旧确认历史告知语义回归测试。
- E2E 最终归档步骤（V3 告知、本人收件、归档 200）。
- 真实数据库/附件联合恢复脚本与 HTTP 夹具。
- 文档状态、权限矩阵、发布清单、部署/恢复说明更新。

PR #9 评审整改（详见 §12）：

- `m6-split-restore.sh` 的退出清理改为安全校验通过后才注册，并只在确实创建隔离库后删除。
- 新增 4 项无破坏性回归测试，证明四类非法环境均不产生任何数据库删除。
- 移除 macOS 专属 `stat -f`/`shasum`，统一使用 Python 3 校验附件大小、SHA-256 与 0700/0600。
- `m6_outsider` 改为只加入第二个 Team，并在源/恢复环境核对真实成员关系与三类 404。

提交 SHA 见最终交付回复与 PR 提交列表；不修改历史 M1–M5 验收报告。

## 9. 遗留风险与技术债

1. **离队成员可信送达**：产品未批准披露/送达通道；缺失收件继续阻塞归档，本轮未降低约束。
2. **无数据库唯一告知约束**：Service + Team→Case 锁保证正常路径，直接 SQL 旁路仍可插入重复；需后续追加迁移/权限治理。
3. **历史告知复用语义**：旧确认请求返回历史 201，未新增 `reused_existing` 契约标识；建议产品确认后调整。
4. **分页/批量/大规模性能**：未实现，未在 M6 声称达标；大量项目/案例下需要契约设计。
5. **生产部署与恢复**：只在一次性隔离环境验证，未做生产存储、备份介质、HTTPS、RTO/RPO 验收。
6. **附件恢复规模**：1 个 108 字节文件，未验证大文件、多文件、跨对象存储场景。
7. **性能客户端**：Python urllib 非生产压测工具；P50/P95 仅供本机参考。
8. **Linux 恢复演练**：脚本已无平台专属命令，但本轮只在 macOS 实际执行，Linux 端到端恢复为 **Not Tested**。
9. **PR 未合并**：M6 仍待评审，V1.1 不能声明发布。

## 10. 发布就绪评估

- **M6 功能验收通过**：是（本报告证据）。
- **V1.1 候选版本具备发布准备条件**：有条件，尚需 M6 PR 合并、生产部署/恢复演练、分页与规模性能决策、离队送达产品决策。
- **生产部署实际验收通过**：否，未执行。
- **main 正式发布完成**：否，未修改 main，未发布。

## 11. 评审 PR

[PR #9](https://github.com/YEXIAONAN/Nexora/pull/9)，目标 `develop`，状态 Open、等待正式评审。不自动合并，不修改 main，不发布 V1.0/V1.1，不进入后续阶段。

## 12. PR #9 评审整改（最终验收）

### 12.1 P1 恢复脚本安全门禁

事实：旧脚本在任何校验之前注册 `trap cleanup EXIT`，而 `cleanup` 无条件执行两次 `DROP DATABASE`。因此错误标签、错误端口、停止容器或缺失凭据时仍会向 `nexora-devtest-pg` 发出删除命令。

修复：

- 四项校验（容器标签、运行状态、loopback 端口、凭据文件）全部通过后才注册退出 trap。
- 新增 `DB_TOUCHED` 标志，只有脚本真正创建隔离库后才允许清理删除。
- 新增 [test_m6_split_restore_safety.py](file:///Users/waiting/Desktop/Project/Nexora/infra/scripts/test_m6_split_restore_safety.py)，用假 `docker` 记录调用并在临时目录复制真实脚本；断言失败场景退出非 0、未进入构建、没有任何 `docker exec` 与 `DROP DATABASE`。该测试接入 `make verify`（`script-safety-test`）。

失败先行证据：修复前运行 4 项测试全部 FAIL，日志明确出现两次 `DROP DATABASE IF EXISTS nexora_m6_source/restore`；修复后 4/4 PASS，`DROP DATABASE` 计数为 0。测试不连接真实开发数据库，也不使用真实 Docker 守护进程。

### 12.2 P2 跨平台兼容

移除 `stat -f '%z'`、`stat -f '%Lp'` 与 `shasum -a 256`，新增 Python 3 帮助函数读取文件大小、SHA-256（流式）与权限位（04o）；附件总量用 `os.walk` 统计。恢复演练在 macOS 重新全量通过；Linux 实际执行未测试（**Not Tested**），代码路径已无平台专属命令。

### 12.3 P2 真实跨 Team 测试

此前 `m6_outsider` 同时加入了被测 Team，404 实际验证的是“Team 成员但非项目成员”。现改为：

- `m6_worker`、`m6_reviewer`、`m6_appraiser` 加入被测 Team。
- `m6_outsider` 只接受第二个 Team 的邀请，绝不加入被测 Team。
- 源环境与恢复环境都通过 `GET /teams/{id}/members` 核对真实成员关系：outsider 不在被测 Team、在第二 Team；worker 在被测 Team。
- 源环境与恢复环境均验证 attachment、case、`contribution-results/mine` 返回 404；恢复后额外验证 outsider 可读第二 Team 成员列表（200）、读被测 Team 成员列表 404。

最终重跑结果：联合恢复 PASS（11.92s，schema v7，16 张业务表行数一致，附件 108 字节 SHA-256 `0d7dc95c…a125`），`make verify` PASS，Go 全量 `-race` PASS，Vitest 209/209 PASS，Playwright 31/31 PASS，恢复脚本安全回归 4/4 PASS。
