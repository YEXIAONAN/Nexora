# Changelog

## [0.1.0-alpha.3] — 2026-11-24

- 本节记录版本内容与实际验收；标签对应提交、最终合并与预发布状态以 [GitHub Releases](https://github.com/YEXIAONAN/Nexora/releases) 的远程记录为准。
- 定位：内部 Alpha 预览；生产部署和真人团队试用尚未验收。本版本不含业务功能与数据库迁移变更，为文档、发布治理与版本标识更新。
- 当前 API 默认版本与 Web 包版本已设为 `0.1.0-alpha.3`。

### 文档与工程改进

- README 由运维长文重构为项目总览：核心功能（团队与项目、任务与交付、贡献与奖金、权限与安全）、技术栈、项目结构、快速开始、权限设计、文档导航与开发验证命令。保留内部 Alpha 与生产未验收声明；移除指向不存在的 `docs/user-guide/` 的失效链接，其余内部链接全部指向真实文件。
- 归档 [Alpha.2 发布收尾审计报告](docs/development/alpha2-release-cleanup-report.md)：记录 main/develop/tag 一致性核验、17 个已合并 PR 分支的安全清理、隔离干净检出的启动冒烟与 `make verify`/`test`/`build` 结果，及本地重复副本的隔离与处置结论。
- 工作区遗留的 25 个 Finder 式 `" 2"` 重复文件逐项比对确认：22 份与已跟踪文件字节一致，3 份为旧版本快照；全部排除出版本库并保留在本地。验证期间实测其中 `.go` 副本会触发 Go 重复符号编译错误，因此全部本地验证在移除副本后的干净工作区执行。

### 迁移与兼容性

- 无新增迁移：迁移链保持 **v1–v11**，已发布迁移文件字节不变，无需升级动作；v9/v10 奖金路由仍不挂载，完整奖金功能仍要求 v11。

### 实际验证记录（2026-11-24）

以下为本轮在真实提交上的执行结果，不以历史阶段报告代替：

- [x] 本地 `make verify` 通过：gofmt/go vet、13 个 Go 包单元测试、前端 lint/format/typecheck、OpenAPI 生成类型一致、32 个文件 285 项 Vitest 测试、生产构建、secret scan（扫描器 8 项回归与源码扫描）、恢复脚本安全测试（4 项）。
- [x] `make e2e` 在一次性测试栈（127.0.0.1:5433 专用库）通过全部 37 项真实 Chrome 回归；不触碰开发数据库。
- [x] [main CI 45422464429](https://github.com/YEXIAONAN/Nexora/actions/runs/45422464429) 与 [develop CI 45422488136](https://github.com/YEXIAONAN/Nexora/actions/runs/45422488136) 六项 Job 全部成功（Backend Go、Windows/macOS 后端构建测试、前端、Secret scan、Containers）。

发布门禁：develop/main 同步至同一最终提交且两分支 CI 全部通过后，才创建 `v0.1.0-alpha.3` 标签与 prerelease；最终提交对应的 CI 以远程 Release 记录为准，不用先前提交的 CI 代替。

生产部署和真人团队试用仍是独立、未完成的验收事项，不用内部 Alpha 或自动化结果替代。

使用与部署：[README](README.md) · [部署说明](docs/operations/deployment.md)。

## [0.1.0-alpha.2] — 2026-11-22

- 本节记录版本内容与实际验收；标签对应提交、最终合并与预发布状态以 [GitHub Releases](https://github.com/YEXIAONAN/Nexora/releases) 的远程记录为准。
- 定位：内部 Alpha 预览；生产部署和真人团队试用尚未验收。真人试用待真实用户参与，不阻碍内部 Alpha。
- 当前 API 默认版本与 Web 包版本已设为 `0.1.0-alpha.2`；版本号更新不代表标签、镜像或远程 Release 已发布。

### 新增与改进

**团队开通与项目协作（Phase 07）**

- 平台管理员创建团队时指定已有活跃用户为 Owner；ROOT 不自动加入团队，不绕过团队、项目或奖金权限。普通注册用户不能自助创建团队。
- Owner/Admin 邀请成员、创建项目并任命 lead；lead 管理项目成员与任务。团队成员资格不自动等于项目成员资格，管理员身份不隐式授予任务管理权。
- 保留最多三级任务、负责人和协作者；叶子任务在「工作进展」中结构化上报工作内容、已完成工作、下一步计划、当前问题与交付证明。
- 执行上报 `actual_progress` 为整数 0–99；执行中有效进度最高 79%，提交交付后待验收为 80%，独立验收通过才为 100%。「交付与验收」不是进展上报；贡献核定继续独立运行。

**奖金 Vue + API 完整流程**

- 具有有效团队与项目成员关系的 Owner/Admin 或 lead 可创建独立 `funding_key` 的「奖金批次」，填写真实资金来源，并指定不同的金额负责人（manager）与独立核定人（reviewer）；二者都不能是创建者，不可自我授权。
- 金额访问采用显式批次角色：「方案编辑」「预算金额」「资格维护」「独立核定」「审计查看」。创建者、ROOT、团队管理者或 lead 身份不自动获得金额权限。
- 从正式贡献确认来源编制人工总预算、来源预算、逐人资格、金额及理由；贡献比例仅供参考，不自动计算奖金。页面输入人民币元，API/数据库使用整数 CNY 分，分别保留来源及全方案未分配余额。
- 「保存人工方案」生成不可变草稿版本；随后提交独立核定、退回修改或独立核定。核定人必须独立于方案编辑者、受益人和来源贡献提案核定人。
- 核定和发布分离：另行「发布正式结果」后，成员在「我的奖金」仅查看本人已发布结果。修订草稿和待审阶段保留原发布结果；新版本独立核定后替换正式版本，另行发布后成员才可查看新结果，历史记录不覆盖。
- 提供「成员汇总与模拟发放」及历史记录；金额取自已发布结果，不可手填或重复记录。**模拟发放不是银行转账，也不证明现实中已付款；出现发放记录后该批次不能修订。**
- 工作流包含团队隔离、成员权限 epoch、乐观并发、请求幂等和审计；仍依赖有效成员关系及服务端授权。

**开发启动与上手**

- `NEXORA_ENV_FILE` 支持指定配置文件；显式路径缺失即报错，不创建或加载默认 `.env`。
- `NEXORA_EXTERNAL_DATABASE=1` 跳过全部 Compose 操作，但仍迁移指定数据库；运行前必须确认目标与备份。
- `NEXORA_API_PORT` / `NEXORA_HTTP_ADDR` 端口必须一致；使用 `/readyz` 检查 API 复用条件，ROOT 初始化状态区分 `true` / `false` / 未知，不提供默认密码。
- 可复用已有服务，`Ctrl+C` 仅停止本次启动的子进程。README 补齐管理员、第二用户、Owner 邀请、lead 成员管理与各奖金角色的操作入口。
- `./start` 是 Bash 脚本，适用于 macOS/Linux，Windows 推荐 WSL；原生 Windows Go 后端可运行，但没有原生 PowerShell 一键脚本。PR #13 已整合，真实 Windows CI 结果见下方验证记录，不将后端构建测试等同于原生 Windows 一键启动。

### 升级与安全边界

- 最终迁移链为 **v1–v11**；已发布 v1–v8 保持原始字节不变。v9 为 `00009_task_progress_reports.sql`；原未发布的奖金 v9 重编号为 v10 `00010_bonus_foundation.sql`；v11 为 `00011_bonus_workflow.sql`。
- API 启动与 `/readyz` 接受 v9–v11，但**仅 v11 挂载全部奖金读写路由**；v9/v10 的奖金路由返回 404。v8 及更早版本须先升级，v12+ 拒绝。完整 Alpha.2 功能应在迁移至 v11 并重启 API 后验证。
- **v11 Down 明确拒绝，不支持一键回滚。** 升级前验证数据库与附件同恢复点备份；恢复到单独审核的目标库及私有附件目录，核对后切换。不要手改迁移记录、清理未知数据或覆盖正在服务的库。
- 生产必须分离 migration owner 和 app 账号。先按[应用数据库角色](docs/operations/application-database-role.md)执行通用授权，再由 migration owner 执行 `bonus_configure_application_role`；配置后不能批量授予所有函数 EXECUTE。重跑通用授权须重新 configure 并核对有效权限。
- app 凭据属于可信后端，当前不是 RLS，不保证数据库凭据完全泄露后仍能保持身份隔离；不向终端用户提供数据库凭据。

### 实际验证记录（2026-11-22）

整合候选 `4067a50507c14b561930ce2b9ecf16e352eaa946` 保留 Phase 06 M1/M2/M3/M4、Windows 修复及 Phase 07 的提交祖先，见 [整合 PR #15](https://github.com/YEXIAONAN/Nexora/pull/15) 与 [Phase 07 PR #16](https://github.com/YEXIAONAN/Nexora/pull/16)。以下是本轮执行结果，不以历史阶段报告代替：

- [x] 隔离 PostgreSQL 17 上的全量 `go test -race -tags integration ./... -count=1`；Go vet/build 通过，Windows 修复涉及的 config/submissions 本地回归通过。
- [x] 前端 lint、format、typecheck、OpenAPI 生成类型一致性、32 个文件的 285 项 Vitest 测试与生产构建通过。
- [x] 最终 schema v11 上全部 37 项真实 Chrome E2E 通过；覆盖管理员指定 Owner、成员邀请、lead 权限、三级任务、结构化进展、独立验收、贡献、完整奖金闭环和跨团队拒绝。移动端检查无横向溢出，导航开关与本人结果正常。
- [x] v8→v11 升级与数据保留、v9/v10 奖金路由关闭、v11 普通应用角色及权限 epoch、临时表触发器攻击防护、独立性、预算、隔离、并发、幂等和发放后禁止修订回归通过；已发布 v1–v8 与 Alpha.1 标签逐字节一致。
- [x] 指定配置与外部数据库模式的真实 `./start`、已有服务复用、Vite API 代理、已有 ROOT 检测与子进程退出清理通过；不删除数据库。
- [x] 停止本次验收写入后，v11 数据库与附件恢复到独立新目标；46 张表 / 717 行完整逐行比对、13 个附件 SHA-256 一致。恢复后的 `/readyz` 为 v11，6 项 Alpha.2 浏览器闭环再次通过。原库与私有备份保留且未提交至 Git。
- [x] secret scan（8 项扫描器回归及源码扫描）、恢复脚本安全测试（4 项）、Bash 语法与 Compose 配置检查通过。
- [x] 全新空库由真实 `./start` 升级至 v11，终端交互设置并确认随机 ROOT 密码，公开状态由 false 变为 true，所设密码登录成功；再次启动保留账号，退出只停止本次进程，数据库保留。
- [x] [远程 CI 45006884386](https://github.com/YEXIAONAN/Nexora/actions/runs/45006884386) 六项全绿：Linux 全量集成/race、真实 Windows/macOS 后端构建与测试、前端、密钥扫描、API/Web Docker 镜像构建。Windows 证据仅覆盖原生 Go 后端，不等于 PowerShell 一键脚本或生产部署验收。

发布门禁：通过整合 PR 保留源分支祖先，不重写共享历史；develop/main 的最终提交 CI 通过后才创建 `v0.1.0-alpha.2` 标签和 prerelease。最终 PR、SHA、CI 与截图证据记录在远程 Release，不用候选 CI 代替发布提交核验。

生产部署和真人团队试用仍是独立、未完成的验收事项，不用内部 Alpha 或自动化结果替代。

使用与部署：[README](README.md) · [部署说明](docs/operations/deployment.md)。历史 Phase 06 M2/M3 文档描述当时的基础/只读阶段，不代表 Alpha.2 当前能力或本轮验收证据。
