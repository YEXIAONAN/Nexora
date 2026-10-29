# Phase 0 验收报告：项目初始化与架构基线

- 日期：2026-10-29
- 范围：仅 Phase 0，不包含认证、团队、项目、任务、贡献与奖金业务
- 结论：工程基线可用，真实 API / PostgreSQL / Compose 链路已验证；GitHub 推送因网络与认证受限未执行

## 一、完成情况

### 1. 仓库与工程结构

- 在空目录初始化 Git，默认分支 `main`，建立 `develop` 集成分支。
- Monorepo：`apps/api`、`apps/web`、`infra`、`docs`、`.github`。
- 根配置：`.gitignore`、`.dockerignore`、`.editorconfig`、`.env.example`、Prettier、Makefile。

### 2. Go API

- Go 1.26.1 + chi + pgx/v5 + goose + slog，分层为 handler / service / repository / domain / infrastructure。
- 启动时校验环境变量：端口、DSN、连接池、超时、日志级别；生产环境拒绝 `NEXORA_MIGRATE_ON_START=true`。
- 请求 ID 透传与校验、结构化访问日志、panic 恢复、安全响应头、统一错误信封。
- 端点：`GET /healthz`、`GET /readyz`、`GET /api/v1/meta`。
- 优雅退出；数据库连接池设置 `application_name` 与 UTC 时区。
- 迁移命令：`up`、`down`、`status`、`version`；生产向下迁移需显式 `-allow-destructive`。

### 3. Vue 3 前端

- Vue 3.5 + TypeScript 严格模式 + Vite 7 + Pinia + Vue Router + Tailwind CSS 4。
- 工作台外壳：响应式侧边栏、顶栏、深浅色主题（浅色 / 深色 / 跟随系统）。
- 系统总览与运行状态页读取真实 API，不使用模拟数据；错误、加载、未就绪状态均有显式呈现。
- OpenAPI 生成类型提交到仓库（`src/api/schema.d.ts`）。

### 4. PostgreSQL 与容器

- PostgreSQL 17；goose 嵌入式 SQL 迁移；基线表 `system_settings`。
- Docker Compose：默认只启动 `postgres`；`--profile full` 增加 `migrate`、`api`、`web`。
- API 镜像多阶段构建、非 root 运行、健康检查；Web 为 Nginx 静态站点并同源代理 API。
- 迁移服务成功退出后才启动 API；API 健康后才启动 Web。

### 5. 运维与文档

- 备份脚本 `pg_dump -Fc` + SHA-256 + 保留策略；恢复默认使用临时库，禁止直接覆盖主库。
- 备份验证脚本使用 `pg_restore --list` 校验。
- CI：Go 格式 / vet / 迁移 / 集成测试；前端 lint / 格式 / 类型 / 测试 / 构建；密钥扫描；镜像构建。
- 文档：产品需求、架构、数据库、权限、API、开发、测试、部署、备份恢复、路线图、ADR 0001–0005。

### 6. 未包含（按阶段边界）

- 用户、认证、会话、ROOT 引导、团队与权限、项目、三级任务、成果验收、贡献核算、奖金分配、通知与导出。
- 生产部署、TLS 终止、云环境与真实域名验证。
- GitHub 远端推送（见第五节）。

## 二、工程验证

以下均为本机实际执行结果，未使用模拟数据或伪造输出。

| 检查项 | 命令 | 结果 |
| --- | --- | --- |
| Go 格式 | `gofmt -l apps/api/cmd apps/api/internal` | 无输出 |
| Go 静态检查 | `go vet -tags=integration ./...` | 通过 |
| Go 单元测试 | `go test ./...` | 全部通过 |
| Go 集成测试 | `NEXORA_DATABASE_URL=... go test -race -tags=integration ./...` | 全部通过（真实 PostgreSQL 17） |
| 前端类型 | `pnpm typecheck` | 通过 |
| 前端 lint | `pnpm lint` | 通过 |
| 前端格式 | `pnpm format:check` | 通过 |
| 前端单元测试 | `pnpm test` | 3 个文件 / 7 个用例通过 |
| 前端构建 | `pnpm build` | 成功，57 个模块；主 JS 114.19 kB（gzip 45.15 kB） |
| 密钥扫描 | `infra/scripts/check-secrets.sh` | 通过 |
| 总门禁 | `make verify` | 退出码 0 |
| Compose 配置 | `docker compose config -q` | 通过 |
| 镜像构建 | API、Web 镜像 | 均构建成功 |
| 容器栈 | `docker compose --profile full up -d --build` | 4 个服务全部 healthy / 迁移成功退出 |
| API 存活 | `GET :8080/healthz` | 200 |
| API 就绪 | `GET :8080/readyz` | 200，database up，schema version 1 |
| API 元数据 | `GET :8080/api/v1/meta` | 200，version 0.1.0，真实 instance_id |
| Web 首页 | `GET :3000/` | 200 |
| Web 代理 | `GET :3000/api/v1/meta`、`/readyz` | 200，JSON 正确 |
| SPA 深链 | `GET :3000/status` | 200，正常渲染 |
| 安全响应头 | `curl -I :3000/` | CSP、X-Frame-Options、nosniff、Referrer-Policy 均存在 |
| 浏览器渲染 | 1440px 浅色 / 深色、390px 移动端 | 真实数据渲染成功，无控制台错误 |
| 备份 | `make backup` | 生成 dump 并通过 `pg_restore --list` |
| 恢复演练 | `make verify-backup` | 临时库恢复成功，installation_id 一致，随后删除临时库 |

### 验证中发现并修复的问题

1. Nginx 的 `add_header` 在声明了自身头的 location 中不再继承，导致首页与静态资源缺失安全响应头。已抽出 `security-headers.conf` 并在相关 location 显式包含。
2. Nginx 仅代理 `/api/`，`/readyz` 被 SPA 回退为 HTML，生产页面报“无法解析的响应”。已改为同时代理 `/api/`、`/healthz`、`/readyz`。
3. 就绪检查在无 detail 时显示“无返回数据”，已改为“检查通过”。
4. 前端就绪判断改为直接信任服务端 `status === 'ready'`。
5. 容器内无法访问 `proxy.golang.org`，API 镜像增加可覆盖的 `GOPROXY` 构建参数（默认 `goproxy.cn`，校验仍由 `go.sum` 保证）。

### 环境限制（如实记录）

- 当前网络无法直连 Docker Hub 与 `github.com:443`；镜像构建通过 DaoCloud 镜像拉取基础镜像，Go 模块通过 `goproxy.cn`。
- 上述措施不影响仓库在正常网络下的构建方式；Dockerfile 的 `GOPROXY` 可覆盖。
- 未执行真实生产部署、TLS、压测与云环境演练，报告不将其列为已完成。

## 三、数据库变化

迁移版本：`1`（`goose_db_version` 记录）。

新增表：

```sql
system_settings (
  key        text primary key,
  value      jsonb not null,
  updated_at timestamptz not null default now()
)
```

初始化数据：

- `installation_id`：UUID，首次迁移生成，用于标识安装实例；`/api/v1/meta` 返回该值。
- 本次环境实际值：`f942dcb4-9ac7-423f-9654-80c0ff48f2b8`。

未创建用户、团队、项目、任务、成果、贡献、奖金相关表；这些属于后续阶段。

## 四、Git 状态

- 当前分支：`main`
- Phase 0 提交：`854eade37f08d1839e8e6126105130fa451b9805`（`chore: initialize nexora v2 engineering baseline`）
- `develop`：与 `main` 同一提交
- 远端：`origin https://github.com/YEXIAONAN/Nexora.git`（已配置，未成功同步）
- 工作区：提交后干净；备份、`.cache`、`node_modules`、`dist` 均被忽略

推送结果：

- `gh auth status`：未登录任何 GitHub 主机。
- `git ls-remote origin`：连接 `github.com:443` 超时（约 75 秒）失败。
- 因此**未执行成功推送**，也未创建 PR；仓库内容仅存在于本地。未使用强制推送，未改写历史，未操作其他仓库。

## 五、风险与问题

1. **无认证即暴露是危险的**：Phase 0 只有诊断端点，没有登录与权限；在 Phase 1 完成前不得部署到公网。
2. **诊断信息**：`/readyz` 暴露数据库与迁移状态，`/meta` 暴露环境与实例 ID；生产应限制在内网或改为受保护接口。
3. **单实例假设**：当前没有多副本一致性、会话、限流与审计表；Phase 1 起补齐。
4. **网络依赖**：镜像基础层与 Go 模块依赖外部源；离线环境需预置镜像与模块缓存。
5. **备份仅在本地演练**：未验证对象存储、异地保留与定时调度；生产需接入平台备份。
6. **健康检查日志噪声**：容器每 15 秒访问 `/healthz` 会写 info 日志；后续可按路由降级为 debug。
7. **前端无 E2E**：当前为组件/Store 单测与手工浏览器验证，尚无 Playwright 级回归。
8. **GitHub 未同步**：本地提交无法作为团队交付基线，需先恢复网络并完成 `gh auth login`。

## 六、下一阶段（Phase 1 计划，尚未开始）

目标：认证、ROOT 安全引导、团队与权限、团队隔离。

1. 数据模型：`users`、`sessions`、`teams`、`team_members`、`audit_events`；不可变用户 ID 与 `system_root` 标记。
2. 密码与会话：Argon2id、密码策略、登录限流、HttpOnly/SameSite Cookie、会话撤销与过期。
3. ROOT 引导：`waiting` 仅作为初始显示标识，不通过昵称授予权限；无默认密码；首次设置一次性引导令牌；ROOT 操作全量审计。
4. 团队：创建、修改、归档、成员邀请/移除、团队角色、团队切换。
5. 授权：后端中间件 + Service 层双重校验；跨团队 IDOR 集成测试；被移除成员会话立即失效。
6. 资格模型：团队成员、参赛资格、奖金资格三者分离，为 Phase 4/5 留出数据基础。
7. 前端：登录页、首次引导、团队切换、成员管理、审计日志只读页；仍不使用假数据。
8. 验收：权限矩阵测试、IDOR 测试、会话失效测试、审计完整性测试、迁移与回滚验证。

Phase 1 需要先解决 GitHub 网络与认证问题，否则代码仍只能停留在本地。
