# ADR 0002：Go 基础依赖选型

- 状态：已接受
- 日期：2026-10-29

## 决策

| 能力 | 选型 | 理由 |
| --- | --- | --- |
| 路由 | `go-chi/chi v5` | 轻量、兼容 `net/http`、中间件模型清晰 |
| 数据库驱动 | `jackc/pgx v5` | PostgreSQL 原生协议、连接池、性能与类型支持 |
| 迁移 | `pressly/goose v3` | SQL 迁移、嵌入二进制、Up/Down/Status 完整 |
| 日志 | 标准库 `log/slog` | 无额外依赖，JSON 结构化输出 |
| ID | `google/uuid` | 请求 ID 与未来业务 ID |

依赖版本锁定在 `go.mod` / `go.sum`。不使用 ORM；SQL 显式编写并由集成测试覆盖。

> 2026-11-17 增补（Phase 02，随方案 B 批准）：新增 `golang.org/x/crypto`（Argon2id 密码哈希）与 `golang.org/x/term`（ROOT 引导无回显读密码），决策与参数见 [ADR 0006](0006-auth-session-security.md)。

## 后果

- 需要手写 SQL 与 Repository 映射。
- 迁移文件不可修改，只能追加。
- goose 的全局配置在测试中串行使用，集成测试不并行执行迁移。
