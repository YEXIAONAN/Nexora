# 系统架构

## 总体形态

**模块化单体后端 + 前后端分离 + 单实例多团队隔离**。

多团队及后续贡献、奖金模块属正式目标架构。**V1.0 已于 2026-11-17 由产品负责人批准采用方案 B**：多 Team 数据与权限架构先行，首版优先交付 ZERO_R 核心业务闭环；V2.0 原始需求完整保留为长期规划（见 PRD）。多 Team 与独立角色设计不被删减，UI 初期单 Team 视角不构成硬编码单团队。当前工程工作代号 Phase 02 开发身份认证、团队与权限（ADR 0004/0006 已接受）；Phase 01 工程基线修复已完成。阶段代号不构成产品范围决策。

不引入微服务、消息队列或 Kubernetes。当前规模下，模块化单体可以用事务保证一致性与部署简单性；模块边界通过包结构与数据库约束维持。

```text
Browser
  │  HTTPS
  ▼
Nginx (静态资源 + /api 反向代理)
  │
  ├── Vue 3 SPA (apps/web)
  │
  └── Go HTTP API (apps/api)
          │
          ├── Handler        参数解析、响应映射
          ├── Service        业务规则、事务边界、授权
          ├── Repository     SQL / pgx
          ├── Domain         实体、状态机、计算规则
          └── Infrastructure 配置、日志、迁移
                │
                ▼
          PostgreSQL 17
```

Phase 0 只实现 Handler、基础设施与只读 Repository；Service/Domain 在业务阶段引入，不为空层创建占位接口。

## 后端分层

| 包 | 职责 | Phase 0 |
| --- | --- | --- |
| `cmd/server` | 进程组装、HTTP 生命周期、优雅退出 | 已实现 |
| `cmd/migrate` | 显式迁移命令 | 已实现 |
| `internal/config` | 环境变量加载与启动期校验 | 已实现 |
| `internal/observability` | JSON 日志、请求 ID、访问日志、panic 恢复、安全响应头 | 已实现 |
| `internal/httpx` | 统一 JSON 与错误信封 | 已实现 |
| `internal/httpapi` | 路由、健康检查、实例元数据 | 已实现 |
| `internal/database` | pgx 连接池、goose 迁移 | 已实现 |
| `internal/system` | 平台级设置的 Repository | 已实现 |
| `internal/reqctx` | 请求上下文值、请求身份 | 已实现（Phase 01/02，已合并） |
| `internal/auth` | 密码哈希、会话、登录限流、CSRF、ROOT 引导、平台角色 | Phase 02 已合并并验证（PR #2 已合并） |
| `internal/teams` | 团队、成员、邀请、团队角色、授权链 | Phase 02 已合并并验证（PR #2 已合并） |
| `internal/audit` | 追加式操作审计（随业务事务写入） | Phase 02 已合并并验证（PR #2 已合并） |
| `cmd/bootstrap` | ROOT 安全初始化命令（交互式，无默认密码） | Phase 02 已合并并验证（PR #2 已合并） |

Phase 02 起按业务模块增加 `internal/<module>`，每个模块内部再按 handler/service/repository/domain 组织，而不是建立全局空壳层；不为尚未开发的业务创建空壳模块。

## 请求生命周期

1. `RequestID`：保留合法入站 `X-Request-Id`，否则生成 UUID；写入上下文与响应头。
2. `AccessLog`：记录方法、路由模板、状态码、字节数、耗时、请求 ID。
3. `Recoverer`：捕获 panic，记录堆栈，返回统一 500，不泄漏内部信息。
4. `SecurityHeaders`：`nosniff`、`DENY`、CSP 等；生产环境追加 HSTS。
5. 认证与防护（Phase 02，ADR 0006）：受保护路由校验会话 Cookie → 服务端会话摘要查询 → 注入请求身份；`/api/v1` 下的变更请求校验 `X-Nexora-CSRF` 自定义头；登录接口按 IP+用户名限流。团队权限在 Service 层按请求实时判定，不在会话中缓存成员关系。
6. 路由处理：成功 `{"data": ...}`；失败 `{"error": {code, message, fields?, request_id}}`。

服务端超时：读头 5s、读 15s、写 30s、空闲 60s；就绪检查内部 3s 超时。退出时等待在途请求，默认 15s。

## 配置

全部通过环境变量注入，启动即校验：

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `NEXORA_ENV` | development | development / test / production |
| `NEXORA_HTTP_ADDR` | 127.0.0.1:8080 | 本机进程默认仅监听 loopback；容器内部显式使用 :8080 |
| `NEXORA_DATABASE_URL` | 必填 | PostgreSQL DSN |
| `NEXORA_LOG_LEVEL` | info | debug / info / warn / error |
| `NEXORA_MIGRATE_ON_START` | false | 仅开发可用；生产为 true 时启动失败 |
| `NEXORA_DB_MAX_CONNS` | 10 | 连接池上限 |
| `NEXORA_SHUTDOWN_TIMEOUT` | 15s | 优雅退出等待 |
| `NEXORA_SESSION_TTL` | 12h | 会话绝对有效期（Phase 02） |
| `NEXORA_LOGIN_MAX_FAILURES` | 10 | 登录失败限流阈值，按 IP+用户名（Phase 02） |
| `NEXORA_LOGIN_WINDOW` | 15m | 登录失败统计窗口（Phase 02） |

生产环境强制显式迁移，避免多个副本同时启动时竞争执行 DDL。Cookie `Secure` 属性由 `NEXORA_ENV` 推导：production 强制开启，不提供关闭开关；开发 HTTP 与生产策略分离（ADR 0006）。

## 多团队隔离

强制规则（ADR 0004 已接受，Phase 02 落地，实现状态见 PRD）：

- 所有团队业务表带 `team_id NOT NULL` 与外键；复合外键 `(team_id, user_id)` 防止跨团队错误挂接。
- 授权在 Service 层执行：认证身份 → 确定资源所属 Team → 成员关系及状态 → 团队角色 → （如适用）项目角色 → 操作权限 → 资源状态。
- Repository 查询必须带 `team_id` 条件，禁止先按资源 ID 查询其他团队数据再做内存判断。
- 响应映射：未认证 401；无团队访问权 404（含 ROOT，避免泄漏存在性）；有访问权但操作权限不足 403；状态冲突 409；参数非法 400/422。
- ROOT 不默认绕过团队业务数据授权；平台级可见性通过显式 `/api/v1/admin/teams` 提供。未来紧急跨团队操作需明确、受限、可审计的例外机制（本阶段未实现）。
- 成员移除、角色变更即时生效（每请求实时查询成员关系）；最后一名有效 owner 受数据库触发器保护。
- 导出、批量操作、附件下载同样经过相同的授权路径（相关能力 Phase 03 起开发）。

详见 [权限矩阵](permissions.md) 与 [ADR 0004](adr/0004-tenancy-authorization.md)。

## 可观测性

- 结构化 JSON 日志输出到 stdout，由容器运行时收集。
- 每个请求一个 `request_id`，错误响应携带同一 ID。
- 健康检查分离：`/healthz` 只验证进程；`/readyz` 验证数据库与 Schema。
- 不记录密码、DSN、令牌；连接诊断仅保留 URL 端点及数据库名，用户信息掩码、全部查询参数移除。URL 解析失败不回显原始输入；数据库错误日志仅保留操作、错误类别及合法 SQLSTATE，原始错误仍供内部 `errors.Is/As` 判断，不输出其文本。

## 失败处理

- 启动失败即退出，不进入半可用状态。
- 数据库不可达时 `/readyz` 返回 503，`/healthz` 保持 200，便于区分重启与依赖故障。
- 迁移失败时命令返回非零退出码，Compose 的 `migrate` 服务失败后 API 不会启动。
- 未预期错误只向客户端返回通用 500，细节写入日志。

## 模块边界

| 模块 | 主要职责 | 阶段 |
| --- | --- | --- |
| auth | 登录、会话、ROOT 引导、平台角色 | Phase 02 已合并并验证（PR #2 已合并） |
| teams | 团队、成员、邀请、团队角色、资格 | Phase 02 已合并并验证（PR #2 已合并） |
| audit | 追加式操作审计 | Phase 02 已合并并验证（PR #2 已合并） |
| projects | 项目、成员、里程碑、归档 | Phase 03 已实现并验证（PR #3 已合并） |
| tasks | 三级任务、状态机、依赖、进度 | Phase 03 已实现并验证（PR #3 已合并） |
| reviews | 成果版本、验收记录 | Phase 04 已合并并验证 |
| contributions | 贡献记录、核定、修订 | V1.1（M1 设计冻结待批准） |
| bonuses | 方案、资格、计算、调整、确认 | V2.0 |
| notifications | 站内通知、浏览器通知 | V2.0 |
| exports | Excel/PDF/ZIP 导出 | V2.0 |

模块之间通过 Service 调用与数据库事务协作，不直接跨模块写表；审计写入通过 `internal/audit` 的 Repository 在各业务事务内完成。

## Phase 03 设计增补（2026-11-17）

项目/任务模块及事务边界见 [Phase 03 设计说明](../development/phase-03-design.md)。本次任务规则优先于前文历史规划；Phase 02 已合并于 develop `c3fb029`。实现验证另见阶段报告。
