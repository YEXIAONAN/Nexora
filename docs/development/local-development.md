# 本地开发

## 前置条件

| 工具 | 版本 |
| --- | --- |
| Go | 1.26+ |
| Node.js | 24+ |
| pnpm | 10+ |
| Python | 3.9+，密钥扫描和扫描器回归仅使用标准库 |
| Docker | 24+，含 Compose v2 |

## 首次启动

```bash
make env          # 生成 .env（开发默认值，不含真实凭据）
make dev-db       # PostgreSQL 17，端口 5432
make migrate-up   # 应用迁移
make bootstrap    # ROOT 安全初始化：交互式设置密码，无默认密码
```

`make bootstrap` 在终端中无回显地读取并确认密码；重复引导会被安全拒绝。
自动化场景可向 `go run ./cmd/bootstrap` 的标准输入提供单行密码（仍须显式提供，无默认值）。

两个终端分别运行：

```bash
make dev-api      # http://127.0.0.1:8080
make dev-web      # http://127.0.0.1:5173
```

## 服务与端口

| 服务 | 默认地址 |
| --- | --- |
| Vue 开发服务器 | http://127.0.0.1:5173 |
| Go API | http://127.0.0.1:8080 |
| PostgreSQL | localhost:5432 |
| 容器化 Web（full profile） | http://localhost:3000 |

Vite 代理 `/api`、`/healthz`、`/readyz` 到 `127.0.0.1:8080`，浏览器同源请求不需要 CORS。

本机 API 默认和 Compose 宿主机发布端口均限制在 loopback；容器内部 API 显式监听 `:8080`。开发 Compose 不是生产部署配置，生产环境要求见部署说明。`make verify` 不执行数据库迁移或写库测试；集成测试必须另行配置独立可丢弃数据库，见测试说明。

## 常用命令

```bash
make migrate-status
make migrate-down        # 开发环境回滚一步
make test
make lint
make build
make verify
make backup
make verify-backup
```

## 环境变量

参考 [.env.example](../../.env.example)。关键项：

- `NEXORA_DATABASE_URL`：PostgreSQL DSN。
- `NEXORA_MIGRATE_ON_START`：仅开发可设为 true；生产必须 false。
- `NEXORA_DB_MAX_CONNS`：连接池上限。
- `NEXORA_SESSION_TTL`：会话绝对有效期，默认 12h。
- `NEXORA_LOGIN_MAX_FAILURES` / `NEXORA_LOGIN_WINDOW`：登录失败限流（默认 10 次 / 15 分钟，按 IP+用户名）。
- `VITE_API_BASE_URL`：前端 API 前缀；留空表示同源。

## 故障排查

**Docker daemon 未运行**

```text
Cannot connect to the Docker daemon
```

启动 Docker Desktop 后重试 `docker info`。

**端口被占用**

```bash
lsof -nP -iTCP:5432 -sTCP:LISTEN
lsof -nP -iTCP:8080 -sTCP:LISTEN
```

通过 `NEXORA_POSTGRES_PORT`、`NEXORA_API_PORT` 调整。

**后端报数据库连接失败**

确认 `make dev-db` 已运行且 `.env` 中的 DSN 与 Compose 凭据一致。生产环境不要使用示例密码。

**迁移版本不一致**

```bash
make migrate-status
```

不要手工修改 `goose_db_version`；修正应通过新增迁移完成。

## 仓库内工具缓存

在受限沙箱中运行时，Go 缓存可能需要指向仓库内（`.cache/`，已被 git 忽略）：

```bash
GOMODCACHE="$PWD/.cache/go-mod" GOCACHE="$PWD/.cache/go-build" go test ./...
```

普通本地开发无需设置。


## Phase 04 本地环境

当前 API 要求 schema v6；启动时只检查版本，NEXORA_MIGRATE_ON_START 默认 false。旧运行栈不会随源码更新自动迁移；在已确认目标和备份后显式 make migrate-up。默认私有附件目录相对 API 工作目录为 .data/attachments，生产必须配置绝对 NEXORA_ATTACHMENT_DIR；目录不能作为 Web 静态资源暴露。Docker 新增专用 attachment_data 卷，不依赖其他项目存储。

集成验证按仓库 README 的 test-db.sh 和 testdb 三项环境护栏执行。make e2e 独占 8081/5173 与 nexora_e2e；不要并发启动两个 E2E 脚本。
