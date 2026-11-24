# Nexora Alpha.2 — Release 收尾与本地环境清理报告

- 执行日期：2026-11-23
- 目标版本：`v0.1.0-alpha.2`（Schema `v11`）
- 目标 Commit：`42f9dc670830b44ddee961e47ff1600ec44aee9b`
- 仓库：`https://github.com/YEXIAONAN/Nexora.git`
- 说明：本报告作为交付产物保留在工作区，**未提交/未推送**，以免改变已发布的 `main` SHA（本任务要求最终 `main` 必须仍为 `42f9dc6`）。

---

## A. GitHub 发布状态

| 项目 | 值 |
| --- | --- |
| `origin/main` | `42f9dc670830b44ddee961e47ff1600ec44aee9b` |
| `origin/develop` | `42f9dc670830b44ddee961e47ff1600ec44aee9b` |
| 两者是否一致 | **是**；`git diff origin/main origin/develop` 为空 |
| 目标 SHA 是否存在 | 是；`git rev-parse 42f9dc6…` 命中 |
| 祖先关系 | `origin/main` 是 `origin/develop` 的祖先，反之亦然（同一提交） |

**同步过程（真实执行记录）：** 首次 `git fetch --prune --tags` 时远程 `develop` 由 `ebf6b0e` 前进到 `42f9dc6`；`origin/main` 本已为 `42f9dc6`。因此未制造任何合并提交。仅将本地 `develop` 以 **fast-forward** 方式推进到 `origin/develop`（`Updating ebf6b0e..42f9dc6`），并设置 upstream。远程无需 push。

**Tag：**
- `v0.1.0-alpha.2`：**annotated tag**，tag 对象 `812b6d5`，解引用到 `42f9dc6`（`refs/tags/v0.1.0-alpha.2^{} = 42f9dc6`），与发布目标一致。
- `v0.1.0-alpha.1`：保留（tag 对象 `0929a21` → `cc88b06`）。
- 未删除任何 Tag，未改动任何 GitHub Release。

**README 检查结果：**
以 `git show origin/main:README.md` 为唯一基准逐项核对，13 项要求全部满足，且 `git diff origin/main:README.md origin/develop:README.md` 为空（两分支完全一致）：

1. 版本标识 `v0.1.0-alpha.2` — 满足（顶部标题）。
2. 内部 Alpha 状态说明 — 满足（“内部 Alpha 预览版，不建议用于生产”）。
3. 依赖与版本 — 满足（Docker/Compose v2、Go 1.26+、Node 22+/CI 24、pnpm 10.x；`corepack prepare pnpm@10.34.5`）。
4. `.env` 配置 — 满足（`cp .env.example .env`，列出关键变量）。
5. `./start` 说明 — 满足（步骤、复用、Ctrl+C 行为、日志位置）。
6. ROOT 初始化 — 满足（第 4 节，无默认密码，`make bootstrap`）。
7. 注册/团队开通/邀请成员 — 满足（第 5、6 节）。
8. Project / Task / 工作进展 / 交付验收 — 满足（第 7、8 节）。
9. 贡献核定与奖金管理 — 满足（第 8、9 节）。
10. 人工奖金方案与贡献比例非自动换算 — 满足（“贡献比例仅作参考，没有自动金额公式”）。
11. 模拟发放不等于真实银行转账 — 满足（“这不是银行转账，也不证明现实中已付款”）。
12. Schema v11 与迁移安全 — 满足（v1–v11、v11 Down 拒绝、备份与恢复要求）。
13. 生产部署与真人团队试用待验收 — 满足（顶部声明 + 开发者章节）。

结论：README 无需修改，**未产生任何仅为制造提交的改动**。

---

## B. PR 和分支清理

### PR 合并状态（`gh pr list --state all`）

PR **#1–#17 全部为 `MERGED`**，无 OPEN PR。其中 #10–#17 覆盖 Phase 05–07（贡献、奖金、交付、支付流程）与 Alpha.2 发布；#17 为 `develop → main`，merge commit 即 `42f9dc6`。

### 删除前校验

对全部 17 个远程分支逐一执行 `git merge-base --is-ancestor origin/<branch> origin/develop`，**全部为祖先**；对应 PR 均已合并（例外见下）。仅在此前提下删除。

### 已删除远程分支（17）

```
delivery/alpha.2
feature/phase02-auth-tenancy
feature/phase03-projects-tasks
feature/phase04-submissions-reviews
feature/phase05-contributions
feature/phase05-m3-contribution-service
feature/phase05-m4-contribution-workspace
feature/phase05-m5-hardening
feature/phase05-m6-acceptance
feature/phase06-m1-bonus-design
feature/phase06-m2-schema
feature/phase06-m3-my-results-readonly
feature/phase06-m4-bonus-ui
feature/phase07-product-remediation
fix/phase1-baseline
fix/phase1-engineering
fix/windows-platform-tests
```

### 已删除本地分支（17）

```
delivery/alpha.2
feature/phase02-auth-tenancy
feature/phase03-projects-tasks
feature/phase04-submissions-reviews
feature/phase05-contributions
feature/phase05-m3-contribution-service
feature/phase05-m4-contribution-workspace
feature/phase05-m5-hardening
feature/phase05-m6-acceptance
feature/phase06-m1-bonus-design
feature/phase06-m2-schema
feature/phase06-m3-my-results-readonly
feature/phase06-m4-bonus-ui
feature/phase07-p0-product-usability   （仅本地、非远程；内容已在 main 中）
feature/phase07-product-remediation
fix/phase1-baseline
fix/phase1-engineering
```

本地删除均用 `git branch -d`（非 `-D`），全部确认已合并。

### 保留分支及原因

| 分支 | 原因 |
| --- | --- |
| `main` | 长期保留（发布主干） |
| `develop` | 长期保留（集成分支，与 main 同 SHA） |

`fix/windows-platform-tests` 只有远程、无本地分支（PR #13 已合并），远程已删除。
`fix/phase1-baseline` 无独立 PR 记录，但已确认其 HEAD 是 `origin/develop` 的祖先且无任何进行中的 PR，内容已包含在 develop 中，故安全删除。

### Worktree 处理

- 清理了 4 条**失效** worktree 记录（`Nexora-m2/-m3/-m4/-p0p`，其目录已不存在，`prunable`），仅移除 Git 元数据，未删除任何实际文件。
- **保留** `/Users/waiting/Desktop/Project/Nexora/.run/alpha2-release-source-20261122`（detached HEAD @ `42f9dc6`，Alpha.2 发布源证据）。

### 清理后远程状态（`git ls-remote --heads origin`）

```
42f9dc670830b44ddee961e47ff1600ec44aee9b  refs/heads/develop
42f9dc670830b44ddee961e47ff1600ec44aee9b  refs/heads/main
```

---

## C. Mac 本地环境

### 原有 Nexora 目录清单

搜索范围（限定深度）：`~/Projects`、`~/Code`、`~/Desktop`、`~/Documents`、`~/Downloads`。

| 路径 | 类型 | Remote | 分支 / HEAD | 工作区 | 备注 |
| --- | --- | --- | --- | --- | --- |
| `/Users/waiting/Desktop/Project/Nexora` | Git 仓库 | `YEXIAONAN/Nexora.git` | `main` @ `42f9dc6` | 25 个未跟踪文件 | 原始工作区；含 `.env`、`.run`、`.cache`、`.screenshots`、`backups` |
| `/Users/waiting/Desktop/Project/Nexora-duplicate-quarantine-20261119-2350` | **非 Git** | — | — | — | 已隔离重复副本：`duplicates.tar.gz`、`files/`、`inventory.txt`（约 1.1M），**保留未动** |

- `~/Projects`、`~/Code`、`~/Documents`、`~/Downloads` 下未发现其他 Nexora 副本。
- 原始仓库 25 个未跟踪文件全部为带 `" 2"` 后缀的重复源码（与 quarantine 中 `inventory.txt` 记录一致），例如 `CHANGELOG 2.md`、`apps/api/internal/database/migrations/00011_bonus_workflow 2.sql`、`apps/web/src/views/* 2.vue` 等。**均未删除**。

### 干净检出

| 项目 | 值 |
| --- | --- |
| 路径 | `/Users/waiting/Desktop/Project/Nexora-alpha2-clean` |
| HEAD | `42f9dc670830b44ddee961e47ff1600ec44aee9b` |
| `origin/main` / `origin/develop` | 均为 `42f9dc6` |
| Tag 指向 HEAD | `v0.1.0-alpha.2` |
| Git 工作区 | 干净（仅 `.env`、`.run/`、`dist/`、`node_modules/` 被 gitignore） |
| 重复文件污染 | 无（未拷贝旧副本/产物/.env） |

### 依赖版本（对照 README）

| 依赖 | 实测 | README 要求 | 结论 |
| --- | --- | --- | --- |
| Docker | 24.0.6 | 任意近期版本 | 满足 |
| Docker Compose | v2.22.0-desktop.2 | v2 | 满足 |
| Go | go1.26.1 darwin/amd64 | 1.26+ | 满足 |
| Node.js | v22.23.3 | 22+（CI 24） | 满足 |
| pnpm | 10.34.5 | 10.x | 满足 |
| GNU Make | 3.81 | — | 可用 |

### 环境配置（隔离）

- 干净检出 `.env` **由 `.env.example` 生成**（未复制旧 `.env`）。
- 为与既有项目隔离，显式设置：
  - `COMPOSE_PROJECT_NAME=nexora_alpha2_clean`
  - `NEXORA_POSTGRES_PORT=55433`，`NEXORA_DATABASE_URL=…@localhost:55433/nexora`
  - `NEXORA_API_PORT=8080`、`NEXORA_WEB_PORT=3000`
- `docker compose --env-file .env config` 确认：
  - 项目名 `nexora_alpha2_clean`
  - 卷 `nexora_alpha2_clean_postgres_data`（**与旧 `nexora_postgres_data` 不同**）
  - 端口 `55433->5432`
- 既有容器未受影响：`nexora-postgres-1`（5432，healthy）、`nexora-alpha2-acceptance-pg`（55432）保持运行。

---

## D. 运行验证

在干净检出 `/Users/waiting/Desktop/Project/Nexora-alpha2-clean` 执行 `./start`（后台运行，日志 `.run/start.out`）。

### 启动结果

- 新建隔离网络 `nexora_alpha2_clean_default`、卷 `nexora_alpha2_clean_postgres_data`、容器 `nexora_alpha2_clean-postgres-1`（`127.0.0.1:55433->5432`，healthy）。
- 迁移按 v1→v11 顺序全部 `OK`，结尾 `goose: successfully migrated database to version: 11`。
- Go API 构建并启动，`/readyz` 就绪。
- Vite 7.3.7 启动于 `http://127.0.0.1:3000/`（首次自动 `pnpm install --frozen-lockfile`）。
- ROOT：检测到 `root_initialized=false`；因非交互终端，脚本按设计**跳过**初始化（**未创建任何默认/未知 ROOT 账号**）。

### Smoke Test（真实响应）

```
GET /healthz        -> {"status":"ok",...}
GET /readyz         -> {"status":"ready","checks":{"database":{"status":"up"},
                        "schema":{"status":"up","detail":"version 11"}},...}
GET /api/v1/meta    -> {"data":{"name":"Nexora","version":"0.1.0-alpha.2",
                        "channel":"alpha","environment":"development",
                        "root_initialized":false,"registration_mode":"open",...}}
GET :3000/          -> HTTP 200
```

数据库校验（隔离库 55433）：

```
goose_db_version:  version_id=11 is_applied=t
                   version_id=10 is_applied=t
                   version_id=9  is_applied=t
```

- Schema = **v11**，PostgreSQL 正常，API 正常，前端可访问，ROOT 状态被正确识别（false）。
- 无重复文件导致的构建冲突。

### Makefile 验证（真实退出码）

| 命令 | 结果 | 关键证据 |
| --- | --- | --- |
| `make verify` | 通过 | Go 单元测试全部 `ok`；前端 `Test Files 32 passed / Tests 285 passed`；`openapi types match`；`secret scan passed`；恢复安全测试 `Ran 4 tests OK` |
| `make test` | exit 0 | Go `ok`…，前端 32 文件 / 285 用例全通过 |
| `make build` | exit 0 | 后端 `go build ./...` + 前端 Vite 生产构建成功（`✓ built`） |

### 未完成 / 未执行的验证

- **集成测试**（`//go:build integration`，需 `TEST_DATABASE_URL`）未执行；`make verify`/`make test` 默认不包含。
- **真实浏览器 E2E**（`make e2e`，需 disposable 测试栈）未执行。
- **Windows 原生**构建/测试未在本机复验（以远程 CI 记录为准）。
- **生产部署**与**真人团队试用**仍未验收（与 README 声明一致）。
- ROOT 交互式初始化未执行（非交互环境，避免创建未知账号）。

启动完成后已优雅停止本次 `./start` 启动的 API/前端（`已停止 API/前端进程`），**PostgreSQL 容器与数据卷保留**（`nexora_alpha2_clean-postgres-1` 仍 healthy，数据不丢）。重启只需在干净目录再次 `./start`。

---

## E. 安全核查

| 核查项 | 结论 |
| --- | --- |
| 是否触碰既有数据库 | **否**。使用独立 compose 项目/卷/端口 55433；未连接 5432（`nexora-postgres-1`）或 55432（acceptance）。 |
| 是否保留旧副本 | **是**。`Nexora-duplicate-quarantine-20261119-2350` 及原工作区 25 个 `" 2"` 文件均保留。 |
| 是否保留历史证据 | **是**。`.screenshots`（15 张 Alpha.2 验收截图）、`.run`（发布源 worktree、发布说明、附件、恢复目录）、`backups`（DB dump + remediation）未动。 |
| 是否删除 Docker 数据卷 | **否**。未删除任何既有卷；仅新建隔离卷。 |
| 是否重置数据库 | **否**。仅在全新隔离库上执行向前迁移。 |
| 是否有未提交文件 | 原工作区：25 个旧重复文件 + 本报告（均未提交）；干净检出：无（仅 gitignore 的 `.env`/`.run`/`dist`/`node_modules`）。 |
| 是否 force push / 改写历史 | **否**。 |
| 是否删除 Tag / GitHub Release | **否**。`v0.1.0-alpha.1`、`v0.1.0-alpha.2` 均保留。 |
| 是否上传 `.env`/密码/Token/备份 | **否**。`.env` 与 `.run/*.env` 仅在本地且被 gitignore。 |
| 外部推送范围 | 仅按授权删除已合并的 17 个远程分支；未推送任何代码/报告。 |
| 是否添加 AI 联合署名 | 否。 |

---

## F. 最终结论

**可以认定干净开发基线：** `/Users/waiting/Desktop/Project/Nexora-alpha2-clean`
- Commit `42f9dc670830b44ddee961e47ff1600ec44aee9b`（= `main` = `develop`）
- Tag `v0.1.0-alpha.2`
- Schema `v11`
- 工作区干净、无重复文件污染
- `./start` 启动与冒烟测试通过，`make verify/test/build` 全部通过

**是否可以开始 Phase 08：** 可以。远程仅保留 `main`/`develop`，版本、Schema、README 与发布标签一致。

**仍需人工处理的事项：**
1. 如需在干净环境登录，请在交互终端运行 `./start`（或 `make bootstrap`）完成 ROOT 初始化——本次刻意未创建账号。
2. 原工作区 25 个 `" 2"` 重复文件与 quarantine 目录的最终去留由人工确认（本次按纪律仅保留并报告，未删除）。
3. 集成测试、真实 Chrome E2E、Windows 原生 CI 未在本机复跑，需以远程 CI/发布记录为准。
4. 生产部署与真人团队试用仍待正式验收，不因本地冒烟通过而视为完成。
5. 本报告未提交；如希望纳入版本库，请在确认后由人工另行提交（会改变 `main` SHA，需另立发布决策）。