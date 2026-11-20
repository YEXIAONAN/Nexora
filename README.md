# Nexora

> **v0.1.0-alpha.1 · Alpha 预览版**
> 适合团队内部试用，不建议用于生产环境。本版本不包含 Phase 06 的新奖金核定功能。

Nexora 是一个面向小型研发团队的 Web 协作应用：管理团队、项目与三级任务，通过成果提交和独立审核形成可追溯的贡献记录。

本轮 P0 交付的重点是**可实际使用的前端产品**：自助注册与登录、管理员用户管理、团队邀请、项目与任务协作、成果审核、设置与导航，以及一条命令启动整个开发环境。

---

## 1. 我需要安装什么

| 依赖 | 版本 | 用途 |
| --- | --- | --- |
| Docker + Docker Compose v2 | 任意近期版本 | 本地 PostgreSQL 17 |
| Go | 1.26+ | 后端 API（也可用 Docker 方式） |
| Node.js | 22+（CI 基线 24） | 前端 Vite |
| pnpm | 10.x | 前端依赖与脚本 |

首次使用如未安装 pnpm：

```bash
corepack enable
corepack prepare pnpm@10.34.5 --activate
```

## 2. 配置环境

```bash
cp .env.example .env
```

开发环境默认值即可直接运行。需要关注：

- `NEXORA_DATABASE_URL`：本地 PostgreSQL 连接串（默认指向 `nexora` 数据库，账号与密码见 `.env.example`）
- `NEXORA_REGISTRATION_MODE`：`open` / `invite` / `closed`，开发环境默认 `open`
- `NEXORA_API_PORT` / `NEXORA_WEB_PORT`：默认 `8080` / `3000`

`./start` 首次运行发现 `.env` 缺失时会自动从 `.env.example` 创建，并提示你检查配置。

## 3. 一键启动

```bash
./start
```

它会依次：

1. 启动（或复用）PostgreSQL 容器；
2. 应用尚未执行的数据库迁移；
3. 若系统尚未初始化管理员，进入交互式安全初始化（不提供默认密码）；
4. 启动 Go API 与 Vite 前端；
5. 输出前端地址、API 地址、运行模式和注册策略。

启动成功后访问：

- 前端：**http://127.0.0.1:3000**
- API：http://127.0.0.1:8080
- 健康检查：http://127.0.0.1:8080/healthz

按 `Ctrl+C` 会停止本次启动的 API 与前端。PostgreSQL 容器与数据会保留，第二次运行 `./start` 不会清空账号、团队、项目和任务。

重复执行是安全的：已在运行的 API/前端会被复用，迁移只前进不回滚，不会重建管理员或修改密码。

### 手动启动（可选）

```bash
make env
make dev-db
make migrate-up
make bootstrap      # 仅首次：交互式设置 ROOT 用户名与密码
make dev-api        # 终端 A
make dev-web        # 终端 B，Vite 默认 5173
```

## 4. 第一次创建管理员

- 使用 `./start`：检测到没有 ROOT 时会在终端提示你设置密码；
- 或显式执行：`make bootstrap`。

用户名默认为 `admin`，可通过 `NEXORA_ROOT_USERNAME` 或 `make bootstrap` 之后的交互调整。系统没有默认密码，也不会把管理员账号共享给普通用户。

## 5. 注册第二个用户

开发环境默认开放注册：

1. 打开 http://127.0.0.1:3000/register
2. 填写用户名、显示名称、可选邮箱、密码（至少 10 个字符）
3. 注册成功后回到登录页，用新账号登录

生产环境如需限制注册，设置 `NEXORA_REGISTRATION_MODE=invite`：只有已被团队邀请的用户名才能注册；`closed` 则完全由管理员在「用户管理」中创建账号。

普通注册用户不会获得平台管理员权限。

## 6. 创建 Team 并邀请成员

1. 登录后进入「团队」；
2. 填写团队名称创建 Team，创建者成为 Owner；
3. 打开「团队成员」，在邀请表单填写用户名；
4. 对方如果还没注册，也可以先邀请：对方用**同一个用户名**注册后，登录即可在「邀请收件箱」看到邀请并接受；
5. 接受后可在顶部切换团队，或在团队页设置当前团队。

团队之间数据隔离；同一用户在不同 Team 可以有不同角色（Owner / Admin / Member）。

## 7. 创建 Project

1. 进入当前团队的「项目」；
2. 点击「新建项目」，填写名称、描述与项目负责人；
3. 创建后立即进入项目工作区，项目也会出现在项目列表与工作台。

项目工作区包含：项目概览、任务树、项目成员、里程碑、进度历史、项目设置，以及贡献核算入口。

## 8. 分配任务与提交/审核成果

任务最多三级：

1. 项目负责人或管理员在「任务树」点击「新建任务」；
2. 选择父任务（可留空）、负责人、优先级、权重和截止时间；
3. 编辑任务可添加协作成员；
4. 被分配者可在「我的任务」看到任务，进入项目后开始任务、记录进度；
5. 叶子任务执行到可提交状态后，在「成果与验收」填写成果说明、证据链接或附件，提交新版本；
6. 项目负责人默认可审核；其他成员需由负责人在「项目成员 → 审核人授权」中授予审核权；
7. 审核人填写审核意见并「通过当前版本」或「驳回当前版本」；
8. 驳回后提交者可查看原因、补正并重新提交；提交者不能审核自己的成果。

贡献记录不是自动计算的：正式验收后，可在项目「贡献核算」中按现有 Phase 05 流程建立案例、形成草案并完成独立核定。本轮未新增奖金逻辑。

## 9. 常见启动失败排查

| 现象 | 处理 |
| --- | --- |
| `docker: command not found` / Docker 未启动 | 启动 Docker Desktop，或自行提供 PostgreSQL 并修改 `NEXORA_DATABASE_URL` |
| 端口 3000 / 8080 被占用 | 修改 `.env` 中的 `NEXORA_WEB_PORT` / `NEXORA_API_PORT`，或停止占用进程 |
| `API requires schema v8–v9` | 先运行 `make migrate-status` 与 `make migrate-up`，不要跳过迁移 |
| 数据库连接失败 | 确认 `docker compose ps` 中 postgres 为 healthy，并核对 `.env` 密码 |
| 登录后立即回到登录页 | 清除站点 Cookie 后重试；确认 API 的 `/healthz` 正常 |
| 注册提示需要邀请 | 当前为 `invite` 模式；先由团队 Owner/Admin 邀请该用户名，或改为 `open` |
| 页面空白 / 控制台 401 | 匿名访问受保护接口属正常；若持续 401，重新登录 |
| 想重置本地环境 | 停止 `./start` 后执行 `docker compose down -v`（**会删除本地数据**，仅在确认无价值时使用） |

日志位置：`.run/api.log`、`.run/web.log`。

---

## 开发者章节

### 技术栈

Go + chi + pgx + goose；PostgreSQL 17；Vue 3 + TypeScript + Vite + Pinia + Vue Router + Tailwind CSS；OpenAPI 3.1 契约。

```text
apps/api/         Go 模块化单体（auth / teams / projects / tasks / submissions / contributions）
apps/web/         Vue 工作台
infra/            容器、脚本、测试库与运维工具
docs/             产品、架构、API、开发与运维文档
```

### 常用命令

```bash
make test           # Go 单元测试 + 前端 Vitest
make lint           # gofmt / go vet / 前端 lint
make build          # 后端与前端构建
make verify         # 完整本地验证门禁（含契约与脚本检查）
make migrate-status # 迁移状态
make migrate-up     # 应用迁移
make e2e            # Playwright 真实栈 E2E（按脚本要求提供测试库）
```

### 数据库与迁移

迁移位于 `apps/api/internal/database/migrations/`，只允许追加，不修改历史迁移。API 启动时要求 schema v8–v9（v8 为 Alpha P0，v9 增加 Phase 06 奖金基础结构）；迁移需显式执行（`./start` 会自动 `up`）。

生产环境必须使用普通应用数据库角色，不使用迁移 owner 凭据，详见 [应用数据库角色](docs/operations/application-database-role.md)。

### 相关文档

- 产品需求：[docs/product/product-requirements.md](docs/product/product-requirements.md)
- API 契约：[docs/api/openapi.yaml](docs/api/openapi.yaml)
- 开发流程与阶段报告：[docs/development](docs/development)
- 运维与备份：[docs/operations](docs/operations)

### 账号与会话安全

- 密码使用 Argon2id 哈希；
- 会话通过 HttpOnly Cookie 维持，不写入 localStorage；
- 变更请求需要 `X-Nexora-CSRF: 1`；
- 停用账号会立即撤销现有会话；
- 平台管理员与团队角色分离，团队权限始终由后端校验。
