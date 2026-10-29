# 数据库设计

## 基本约定

- PostgreSQL 17，单一主数据库。
- 主键：业务表使用 `uuid`（`gen_random_uuid()`）或 `bigint generated always as identity`；审计与流水表优先 `bigint`。
- 时间：`timestamptz`，全部以 UTC 写入；连接设置 `timezone=UTC`。
- 金额：最小货币单位 `bigint`（如分），或 `numeric(20,2)`；禁止浮点。
- 布尔：`boolean NOT NULL DEFAULT false`。
- 文本：明确长度约束；状态使用 `text` + `CHECK` 约束（便于演进）或枚举表。
- 外键：默认 `ON DELETE RESTRICT`；核心审计记录不允许级联删除。
- 索引：外键列、团队隔离列、常用排序与过滤组合建立索引；以真实查询为依据。
- 软删除：业务记录使用 `archived_at` / `cancelled_at` 等状态字段，不物理删除历史。

## 迁移

工具：`goose`，SQL 文件嵌入二进制。

- 目录：`apps/api/internal/database/migrations`
- 命名：`00001_baseline.sql`、`00002_create_users.sql`
- 每个文件包含 `-- +goose Up` 与 `-- +goose Down`
- 已发布的迁移不可修改；修正通过新迁移完成
- 生产环境显式执行：`nexora-migrate -command up`
- 回滚：`nexora-migrate -command down -steps N`；生产需 `-allow-destructive`
- 不使用 ORM 自动同步结构

## Phase 0 Schema

### `system_settings`

| 列 | 类型 | 约束 |
| --- | --- | --- |
| `key` | text | 主键，非空且去除空白后非空 |
| `value` | jsonb | NOT NULL |
| `updated_at` | timestamptz | NOT NULL DEFAULT now() |

基线迁移写入 `installation_id`（UUID 字符串），作为部署实例的稳定标识，由 `GET /api/v1/meta` 返回。该表不承载团队数据。

`goose_db_version` 由 goose 自动维护，`/readyz` 读取其中最高已应用版本。

## 计划中的核心表（Phase 1–5）

以下为设计方向，尚未创建；实现前会补充字段、约束、索引与迁移。

| 领域 | 表 | 关键约束 |
| --- | --- | --- |
| 身份 | `users`、`sessions`、`platform_roles` | `users.system_root boolean` 唯一真值；ROOT 唯一部分索引 |
| 审计 | `audit_events` | 追加写；actor、action、target_type、target_id、team_id、before/after、request_id |
| 团队 | `teams`、`team_members` | `(team_id,user_id)` 唯一；角色枚举 |
| 项目 | `projects`、`project_members`、`milestones` | `team_id` 外键；负责人必须是团队成员 |
| 任务 | `tasks`、`task_collaborators`、`task_dependencies` | `depth <= 3`；父子同项目；依赖唯一且无自环 |
| 成果 | `task_submissions`、`task_submission_versions`、`task_reviews` | 版本不可变；验收引用具体版本 |
| 贡献 | `contributions`、`contribution_revisions` | 已确认记录不覆盖；修订保留前后值 |
| 奖金 | `bonus_plans`、`bonus_allocations`、`bonus_adjustments`、`bonus_confirmations` | 金额合计校验；方案版本与状态机 |

### 关键约束思路

- **ROOT 唯一性**：`CREATE UNIQUE INDEX ... ON users (system_root) WHERE system_root`。
- **任务层级**：写入时由 Service 计算 `depth` 并校验 `<= 3`；数据库触发器可作为第二道防线。
- **循环依赖**：依赖写入时在事务内做递归查询校验。
- **并发**：状态转换与金额确认使用 `SELECT ... FOR UPDATE` 或乐观版本列 `version`。
- **审计**：业务状态变化与审计事件在同一事务写入。

## 备份

数据库备份与 Git 仓库分离。备份、恢复与验证流程见 [备份与恢复](../operations/backup-restore.md)。
