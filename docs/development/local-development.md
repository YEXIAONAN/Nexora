# 本地开发

## 前置条件

| 工具 | 版本 |
| --- | --- |
| Go | 1.26+ |
| Node.js | 24+ |
| pnpm | 10+ |
| Docker | 24+，含 Compose v2 |

## 首次启动

```bash
make env          # 生成 .env（开发默认值，不含真实凭据）
make dev-db       # PostgreSQL 17，端口 5432
make migrate-up   # 应用迁移
```

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
