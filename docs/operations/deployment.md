# 部署说明

> 本文件的 Compose 是开发配置。历史本地验收不能替代当前版本验证；Docker 不可用时必须报告未验证完整启动。尚无生产部署、TLS 终止或云环境验收。

## 本地/单机 Docker Compose

```bash
cp .env.example .env
# 修改数据库密码与端口；不要使用示例密码
docker compose --profile full up -d --build
```

服务：

| 服务 | 说明 | 默认端口 |
| --- | --- | --- |
| postgres | PostgreSQL 17，数据卷 `postgres_data` | 127.0.0.1:5432 |
| migrate | 一次性迁移任务，成功后 API 启动 | — |
| api | Go API，非 root 用户 | 127.0.0.1:8080 |
| web | Nginx + 构建后的 SPA，代理 `/api`、`/healthz`、`/readyz` | 127.0.0.1:3000 |

启动顺序由 `depends_on` 条件保证：

```text
postgres healthy → migrate completed → api healthy → web
```

验证：

```bash
curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:8080/readyz
curl -fsS http://localhost:8080/api/v1/meta
curl -fsS http://localhost:3000/ >/dev/null
```

## ROOT 初始化（Phase 02）

全新安装完成迁移后，必须显式初始化 ROOT 账号；不存在默认密码，`waiting` 只是默认用户名标识：

```bash
# 本机开发
make bootstrap

# 容器环境（交互式终端，无回显读取并确认密码）
docker compose --profile full exec api nexora-bootstrap -username waiting
```

- 重复引导会被安全拒绝（数据库部分唯一索引保证全库仅一个 ROOT）。
- 引导事件写入追加式审计 `root.bootstrap`。
- 自动化可向命令标准输入提供单行密码；仍须显式提供，命令不生成任何默认凭据。
- `GET /api/v1/meta` 的 `root_initialized` 字段供前端展示初始化状态。
- ROOT 恢复流程尚未实现；不得通过更改用户名或普通接口修改 `system_root`。

## 生产注意事项

1. **认证已合并，生产环境仍须单独验收**：必须部署 HTTPS、强凭据、反向代理和运行权限边界。开发 Compose 固定 `NEXORA_ENV=development`，不应直接当作生产配置。
2. **TLS**：在反向代理或负载均衡器终止 TLS；HSTS 仅在 `NEXORA_ENV=production` 时由 API 返回。
3. **密钥**：数据库密码、会话密钥通过部署平台注入，不写入镜像或仓库。
4. **迁移**：生产禁止 `NEXORA_MIGRATE_ON_START=true`（配置校验会拒绝启动）；部署流程先运行 `nexora-migrate -command up`，再滚动更新 API。
5. **多副本**：迁移只由一个 Job 执行；API 副本无状态，可水平扩展。
6. **数据库**：托管 PostgreSQL 建议开启自动备份与时间点恢复，并独立执行本文档的恢复演练。
7. **日志**：容器 stdout JSON 日志接入采集系统；按 `request_id` 关联请求。
8. **健康检查**：`/healthz` 用于重启判断；`/readyz` 用于流量接入。不要把 `/healthz` 当作依赖检查。

本机 Go 默认监听 `127.0.0.1:8080`；Compose 的 API 在容器内显式监听 `:8080`，宿主机发布端口均限制到 loopback。生产应通过独立、经审查的配置显式注入 `NEXORA_ENV=production`、数据库连接、受控监听地址和真实凭据，配置数据库 TLS 与入口 HTTPS，禁止复用示例密码或自动迁移。当前没有提供或执行生产发布。

## 构建参数

API 镜像支持：

```bash
docker build -f infra/docker/api.Dockerfile \
  --build-arg VERSION=2.0.0 \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -t nexora-api:2.0.0 .
```

构建信息通过 `GET /api/v1/meta` 暴露，不包含敏感配置。

若构建环境无法访问 `proxy.golang.org`，可覆盖模块代理（`go.sum` 仍负责校验内容）：

```bash
docker build -f infra/docker/api.Dockerfile \
  --build-arg GOPROXY=https://goproxy.cn,direct \
  -t nexora-api:2.0.0 .
```

Web 镜像构建参数 `VITE_API_BASE_URL` 默认为空，表示同源 `/api`。

## 升级与回滚

1. 备份数据库并验证（`make backup && make verify-backup`）。
2. 应用向前迁移。
3. 部署新镜像，观察 `/readyz` 与错误日志。
4. 应用回滚需要数据库向下迁移时，先确认迁移提供可逆的 `Down`，并在生产显式传入 `-allow-destructive`。

不提供未经验证的“一键回滚”脚本。


## Phase 04 部署增补

本次只构建镜像和验证开发配置，不升级运行中的旧栈。新 API 要求 schema v6；生产禁止启动自动迁移，维护窗口显式执行已评审迁移。生产必须提供私有绝对 NEXORA_ATTACHMENT_DIR 并赋予 API 用户写权限；Docker 镜像创建 /app/.data/attachments，Compose 使用独立 attachment_data 卷。数据库和文件需同恢复点备份；单机目录方案不支持直接横向扩容，不提供病毒扫描。禁止将附件目录挂到 nginx 的静态站点路径。
