# Nexora

Nexora 是 ZERO_R 等小型研发团队使用的内部工程管理平台。**V1.0 方案 B 已于 2026-11-17 批准**：先建立多 Team 数据与权限边界，首版交付项目、任务、成果与独立验收闭环。正式范围见 [产品需求](docs/product/product-requirements.md)。

Phase 01–04 与 Phase 05 M2–M5 已合入 `develop`；Phase 03 合并提交为 `c360327`（[PR #3](https://github.com/YEXIAONAN/Nexora/pull/3)），Phase 04 为 `07f6fa1c`（[PR #4](https://github.com/YEXIAONAN/Nexora/pull/4)），M3 为 `d030198`（PR #6），M4 为 `9c8b0e5`（PR #7），M5 为 `9e35a28`（PR #8）。Phase 05 M6 综合验收在 `feature/phase05-m6-acceptance` 进行，见 [M6 报告](docs/development/phase-05-m6-report.md)。V1.0/V1.1 尚未声明发布；`main` 仍保留旧发布基线，发布同步见 [检查清单](docs/development/main-readme-release-checklist.md)。

## 产品版本与功能

| 产品版本 | 范围 | 状态 |
| --- | --- | --- |
| V1.0 | 方案 B：多 Team、项目、三级任务、成果版本和正式验收 | 已批准，按阶段开发；尚未声明发布 |
| V1.1 | 验收后的贡献记录、建议与人工核定 | Phase 05 M2–M5 已合并；M6 综合验收评审中，尚未声明发布 |
| V2.0 | 奖金方案、精确分配与更完整的协作工具 | 长期规划，未作为当前交付 |

| 能力 | 阶段 | 当前交付位置 |
| --- | --- | --- |
| 工程基线、诊断、CI、备份脚本 | Phase 01 | 已合并 develop |
| ROOT 初始化、账号、会话、Team、RBAC、审计 | Phase 02 | 已合并 develop |
| 项目、成员、交接、归档、里程碑、三级任务 | Phase 03 | 已合并 develop |
| 权重进度、依赖、追加式历史与快照 | Phase 03 / 04 | 已合并 develop |
| 不可变成果 V1/V2、链接、Commit/PR 引用 | Phase 04 | 已合并 develop（PR #4） |
| 私有附件、审核人授权、独立验收与重新提交 | Phase 04 | 已合并 develop（PR #4） |
| 贡献 Schema v7、状态机、争议与归档完整性 | Phase 05 M2 | 已合并 develop（PR #5 / b336bed）；见 [M2 报告](docs/development/phase-05-m2-report.md) |
| 认证贡献服务、核定/争议 API、本人显式收件 | Phase 05 M3 | 已合并 develop（PR #6 / d030198）；见 [M3 报告](docs/development/phase-05-m3-report.md) |
| 贡献 Vue 工作台、人工草案、争议与隐私 intake | Phase 05 M4 | 已合并 develop（PR #7 / 9c8b0e5）；见 [M4 报告](docs/development/phase-05-m4-report.md) |
| 告知业务幂等、前端请求去重与取消传播 | Phase 05 M5 | 已合并 develop（PR #8 / 9e35a28）；见 [M5 报告](docs/development/phase-05-m5-report.md) |
| 综合验收、联合备份恢复与对抗性测试 | Phase 05 M6 | [PR #9](https://github.com/YEXIAONAN/Nexora/pull/9) 待评审；见 [M6 报告](docs/development/phase-05-m6-report.md) |
| 奖金、完整通知和导出 | 后续阶段 | 未交付 |

## 技术与环境

Go 1.26.1+、chi、pgx、goose、PostgreSQL 17；Vue 3、TypeScript、Vite、Pinia、Vue Router、Tailwind CSS。接口采用 OpenAPI 3.1，前端类型由契约生成。需要 Node.js 24（CI 基线，包允许 Node 22+）、pnpm **10.34.5**、Python 3.9+、Docker 与 Compose v2；浏览器 E2E 使用 Chrome。

```text
apps/api/         Go 模块化单体
apps/web/         Vue 工作台与 Vitest / Playwright
infra/            容器、迁移验证、测试库、备份与扫描脚本
docs/             产品、架构、API、开发及运维文档
```

## 安装与本地启动

以下命令用于**新建本地开发环境**。已有环境先确认 `.env` 的目标库并备份，再显式迁移。运行中的旧开发栈不一定应用了最新迁移；本次开发不会替它升级或重启。M3 API 要求 schema v7，其他版本拒绝启动；不会自动升级旧库。生产 API 还要求通过[普通应用角色检查](docs/operations/application-database-role.md)，不得使用迁移 owner 凭据。

```bash
corepack enable
corepack prepare pnpm@10.34.5 --activate
pnpm --dir apps/web install --frozen-lockfile
make env
# 编辑 .env 的数据库地址与密码。NEXORA_MIGRATE_ON_START 默认 false。
make dev-db
make migrate-status
make migrate-up
make bootstrap             # 交互设置 ROOT，无默认账号密码
make dev-api               # 终端 A，127.0.0.1:8080
make dev-web               # 终端 B，localhost:5173
```

ROOT 初始化只允许一次；`waiting` 不是默认登录凭据。首次初始化后使用设置的账号密码登录。Vite 将 `/api`、`/healthz`、`/readyz` 代理到本地 API；可用 `VITE_DEV_API_TARGET` 指定独立 API。

```bash
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/readyz
curl -fsS http://127.0.0.1:8080/api/v1/meta
```

全新、已确认目标的本地容器环境可用 `make compose-up`（Web 3000 / API 8080 / PostgreSQL 5432）；它包含显式 migrate 服务，会升级其配置的数据库，**不要对未经确认的旧栈直接执行**。`docker compose config --quiet` 可先校验配置。

## 从团队到成果验收

1. ROOT 在成员管理入口创建内部账号；当前不开放公众注册。登录后创建 Team，再邀请账号加入，受邀者接受邀请。
2. Team owner/admin 创建项目并指定项目负责人。项目负责人添加有效 Team 成员，创建最多三级的任务，分配负责人、协作者、叶子权重和前置依赖。
3. 负责人或协作者开始未阻塞叶子任务，记录实际进度。执行有效进度上限 79%；父任务只汇总后代叶子。
4. 任务负责人或有效协作者填写成果说明、HTTP(S) 链接、Commit/PR 引用或真实附件，提交不可变 V1。Git 引用只是证据地址，不自动验证仓库内容。提交后为待验收，计 80%。
5. 项目负责人默认能审核；其他有效项目成员须在“项目成员 → 审核人授权”获得明确授权。Team 管理角色、ROOT 身份及任务参与本身不赋予审核权；任何人都不能审核自己的提交。
6. 审核人在项目列表的“待我审核”打开当前版本，填写意见后通过或驳回。驳回计 0%，旧版本与意见保留；提交者可带原因进入补正，或直接提交新 V2。V2 必须独立审核。
7. 当前版本通过后任务正式完成，计 100%，后续依赖解除；取消不解除依赖，且不参与权重分母。完成、取消是终态，普通更新不能跳过审核。项目归档后只读。

## 验证与独立测试数据库

```bash
make verify      # 格式、静态检查、单元、构建、OpenAPI 类型及密钥扫描
```

集成测试**禁止连接普通 nexora 开发库或生产库**。脚本只管理带专用标签的 `nexora-devtest-pg`，使用 loopback **5433**、tmpfs 数据和随机密码；不复用运行中的 5432 开发服务。

```bash
bash infra/scripts/test-db.sh start
NEXORA_ENV=test NEXORA_TEST_DB_DISPOSABLE=1 \
TEST_DATABASE_URL="postgres://nexora:$(cat .cache/devtest-pg-password)@127.0.0.1:5433/nexora_test?sslmode=disable" \
  go -C apps/api test -race -count=1 -tags=integration ./...
make e2e
bash infra/scripts/test-db.sh stop
```

`testdb` 仍强制 test 环境、disposable 标志、loopback 和 `nexora_test*` / `nexora_e2e*` 库名；每个集成测试创建并清理独立 schema。E2E 仅创建/删除 `nexora_e2e`，使用独立 API 8081、前端 5173、随机 ROOT 和临时附件目录；运行前保持端口空闲。脚本拒绝操作无专用标签的同名容器。凭据只在被忽略的 `.cache`，不要打印或提交。

## 私有附件与部署边界

单实例本地私有文件存储：开发默认相对 API 工作目录的 `.data/attachments`，生产必须显式设置绝对 `NEXORA_ATTACHMENT_DIR`。目录 0700、文件 0600，服务端随机键、禁止覆盖与目录逃逸。支持 UTF-8 txt/md/csv、PDF、PNG/JPEG；单文件 10 MiB、版本最多 5 个/25 MiB、Team 未物理移除附件合计 512 MiB。暂存 24 小时后由启动及每小时回收处理；已绑定版本的文件不自动删除。下载重新授权并强制 attachment、octet-stream、nosniff、私有缓存和 sandbox。

不提供病毒扫描或多副本文件同步保证。生产要使用 HTTPS、独立数据库账号与强密码，保护 Cookie 与私有目录；不要暴露 PostgreSQL。Docker Compose 默认值仅适合本地开发，API 使用独立 `attachment_data` 卷；本次没有复用历史 SeaweedFS。多实例部署须先引入经过评估的私有对象存储与一致性设计。

数据库备份命令仍是 `make backup` / `make verify-backup`，**它们只覆盖数据库**。Phase 04 必须在暂停业务写入的窗口同时备份数据库和私有附件目录并记录相同恢复点；具体恢复步骤见 [备份与恢复](docs/operations/backup-restore.md)。v6 在存在成果或附件证据时拒绝 down；v7 存在贡献案例或授权时也拒绝破坏性 down。回退应用需使用验证过的匹配备份。

## 文档与 API

- [PRD](docs/product/product-requirements.md)、[路线图](docs/development/roadmap.md)、[本地开发](docs/development/local-development.md)、[测试](docs/development/testing.md)
- [数据库](docs/architecture/database.md)、[权限](docs/architecture/permissions.md)、[ADR 0004](docs/architecture/adr/0004-tenancy-authorization.md)、[ADR 0005](docs/architecture/adr/0005-money-and-progress.md)
- [OpenAPI](docs/api/openapi.yaml)、[API 说明](docs/api/README.md)、[Phase 04 设计](docs/development/phase-04-design.md)、[验收报告](docs/development/phase-04-report.md)
- [部署](docs/operations/deployment.md)、[备份恢复](docs/operations/backup-restore.md)、[main 发布检查清单](docs/development/main-readme-release-checklist.md)

采用 Conventional Commits 和功能分支 PR；当前阶段合入 `develop`，不直接修改 `main`、不自动合并。完整约定见 [CONTRIBUTING.md](CONTRIBUTING.md)。许可尚未确定，仓库暂未授予外部使用许可。
