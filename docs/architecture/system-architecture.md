# 系统架构

## 总体形态

**模块化单体后端 + 前后端分离 + 单实例多团队隔离**。

以上多团队及后续贡献、奖金模块为历史目标架构。当前 V1.0 优先 ZERO_R 单团队的任务、成果及验收；Phase 01 仅修复工程基线，Phase 02 才开发身份认证与权限，不在本阶段增加业务模块。

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
| `internal/reqctx` | 请求上下文值 | 已实现 |

Phase 1 起按业务模块增加 `internal/<module>`，每个模块内部再按 handler/service/repository/domain 组织，而不是建立全局空壳层。

## 请求生命周期

1. `RequestID`：保留合法入站 `X-Request-Id`，否则生成 UUID；写入上下文与响应头。
2. `AccessLog`：记录方法、路由模板、状态码、字节数、耗时、请求 ID。
3. `Recoverer`：捕获 panic，记录堆栈，返回统一 500，不泄漏内部信息。
4. `SecurityHeaders`：`nosniff`、`DENY`、CSP 等；生产环境追加 HSTS。
5. 路由处理：成功 `{"data": ...}`；失败 `{"error": {code, message, fields?, request_id}}`。

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

生产环境强制显式迁移，避免多个副本同时启动时竞争执行 DDL。

## 多团队隔离

计划中的强制规则（Phase 1 落地）：

- 所有团队业务表带 `team_id NOT NULL` 与外键。
- 授权在 Service 层执行：身份 → 团队成员关系 → 团队角色 → 项目角色 → 操作权限。
- Repository 查询必须带 `team_id` 条件，禁止先按资源 ID 查询再做内存判断。
- 跨团队访问统一返回 404，避免泄漏资源存在性。
- 导出、批量操作、附件下载同样经过相同的授权路径。

详见 [权限矩阵](permissions.md)。

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

## 未来模块边界

| 模块 | 主要职责 |
| --- | --- |
| auth | 登录、会话、ROOT 引导、平台角色 |
| teams | 团队、成员、邀请、团队角色、资格 |
| projects | 项目、成员、里程碑、归档 |
| tasks | 三级任务、状态机、依赖、进度 |
| reviews | 成果版本、验收记录 |
| contributions | 贡献记录、核定、修订 |
| bonuses | 方案、资格、计算、调整、确认 |
| notifications | 站内通知、浏览器通知 |
| exports | Excel/PDF/ZIP 导出 |

模块之间通过 Service 调用与数据库事务协作，不直接跨模块写表。
