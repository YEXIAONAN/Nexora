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

## Phase 02 Schema（已合并并验证）

迁移 `00002_auth.sql` 与 `00003_teams.sql` 追加于版本 1 之后；不修改已发布的 `00001_baseline.sql`。两个迁移已在全新数据库与版本 1 升级路径上运行，并在一次性测试库完成 up/down/up 回滚验证（2026-11-17）；未触碰运行中的开发数据库。

### `users`

| 列 | 类型 | 约束 |
| --- | --- | --- |
| `id` | uuid | 主键，默认 `gen_random_uuid()` |
| `username` | text | NOT NULL UNIQUE，`CHECK (username ~ '^[a-z0-9_-]{3,32}$')`（服务层统一小写） |
| `display_name` | text | NOT NULL DEFAULT ''，长度 ≤ 64 |
| `password_hash` | text | NOT NULL，Argon2id PHC 格式，非空白 |
| `system_root` | boolean | NOT NULL DEFAULT false；部分唯一索引 `users_single_system_root ON users (system_root) WHERE system_root` 保证全库唯一 ROOT |
| `created_at` / `updated_at` | timestamptz | NOT NULL DEFAULT now() |

### `sessions`

| 列 | 类型 | 约束 |
| --- | --- | --- |
| `id` | uuid | 主键 |
| `user_id` | uuid | NOT NULL，FK → users，ON DELETE RESTRICT |
| `token_hash` | text | NOT NULL UNIQUE，`CHECK (~ '^[0-9a-f]{64}$')`（SHA-256 hex，不存原始令牌） |
| `user_agent` | text | NOT NULL DEFAULT ''，≤ 256 |
| `created_at` / `expires_at` | timestamptz | NOT NULL；`CHECK (expires_at > created_at)` |
| `revoked_at` | timestamptz | NULL；`CHECK (revoked_at IS NULL OR revoked_at >= created_at)` |

索引：`(user_id)`、`(expires_at)`。有效会话判定：`expires_at > now() AND revoked_at IS NULL`。

### `platform_roles`

| 列 | 类型 | 约束 |
| --- | --- | --- |
| `user_id` | uuid | NOT NULL，FK → users RESTRICT；与 role 组成主键 |
| `role` | text | NOT NULL，`CHECK (role IN ('platform_admin'))` |
| `granted_by` | uuid | NOT NULL，FK → users RESTRICT |
| `created_at` | timestamptz | NOT NULL DEFAULT now() |

ROOT 不经此表（`system_root` 唯一真值）；禁止向 ROOT 用户授予平台角色（服务层 409）。

### `audit_events`

| 列 | 类型 | 约束 |
| --- | --- | --- |
| `id` | bigint | GENERATED ALWAYS AS IDENTITY 主键 |
| `actor_id` | uuid | NULL（登录失败等无身份事件），FK → users RESTRICT |
| `action` | text | NOT NULL，非空白 |
| `target_type` / `target_id` | text / uuid | NULL |
| `team_id` | uuid | NULL；FK → teams（在 00003 中追加）RESTRICT |
| `request_id` | text | NOT NULL DEFAULT '' |
| `details` | jsonb | NOT NULL DEFAULT '{}'（前后状态、原因；禁止密码/Cookie/原始令牌） |
| `created_at` | timestamptz | NOT NULL DEFAULT now() |

索引：`(team_id, created_at)`、`(actor_id, created_at)`、`(target_type, target_id)`、`(created_at)`。**追加写**：触发器 `audit_events_no_mutation` 禁止 UPDATE/DELETE。业务变更与审计同事务。

### `teams`

| 列 | 类型 | 约束 |
| --- | --- | --- |
| `id` | uuid | 主键 |
| `name` | text | NOT NULL，`CHECK (length(btrim(name)) BETWEEN 1 AND 64)` |
| `description` | text | NOT NULL DEFAULT ''，≤ 500 |
| `created_by` | uuid | NOT NULL，FK → users RESTRICT |
| `created_at` / `updated_at` | timestamptz | NOT NULL DEFAULT now() |
| `archived_at` | timestamptz | NULL（归档能力后续阶段实现，列先行保留） |

### `team_members`

| 列 | 类型 | 约束 |
| --- | --- | --- |
| `team_id` | uuid | NOT NULL，FK → teams RESTRICT；与 user_id 组成主键 |
| `user_id` | uuid | NOT NULL，FK → users RESTRICT |
| `role` | text | NOT NULL，`CHECK (role IN ('owner','admin','member'))` |
| `status` | text | NOT NULL DEFAULT 'active'，`CHECK (status IN ('active','removed'))` |
| `created_at` / `updated_at` | timestamptz | NOT NULL DEFAULT now() |
| `removed_at` | timestamptz | NULL；`CHECK` 保证 active↔NULL、removed↔NOT NULL 一致 |

索引：`(user_id, status)`（我的团队列表）、部分索引 `(team_id) WHERE status='active' AND role='owner'`（最后 owner 计数）。
触发器 `team_members_last_owner_guard`：拒绝使团队失去最后一名 active owner 的 UPDATE/DELETE；服务层成员/角色变更前对 `teams` 行 `FOR UPDATE` 串行化，保证并发下触发器计数正确。
主键 `(team_id, user_id)` 同时是未来跨表关联的复合外键目标（如项目成员引用 `(team_id, user_id)`），防止跨 Team 错误挂接。

### `team_invitations`

| 列 | 类型 | 约束 |
| --- | --- | --- |
| `id` | uuid | 主键 |
| `team_id` | uuid | NOT NULL，FK → teams RESTRICT |
| `invited_user_id` | uuid | NOT NULL，FK → users RESTRICT（仅限已存在账号） |
| `invited_by` | uuid | NOT NULL，FK → users RESTRICT |
| `role` | text | NOT NULL，`CHECK (role IN ('admin','member'))`（owner 不可经邀请产生） |
| `status` | text | NOT NULL DEFAULT 'pending'，`CHECK (status IN ('pending','accepted','declined','cancelled'))` |
| `expires_at` | timestamptz | NOT NULL，`CHECK (expires_at > created_at)`；过期为时间派生态，接受时校验 |
| `created_at` / `updated_at` | timestamptz | NOT NULL DEFAULT now() |
| `responded_at` | timestamptz | NULL；`CHECK` 保证 pending↔NULL、非 pending↔NOT NULL 一致 |

索引：部分唯一索引 `(team_id, invited_user_id) WHERE status='pending'`（重复邀请防护）、`(invited_user_id, status)`、`(team_id, status)`。
接受流程：事务内对邀请行与团队行加锁，校验 pending/未过期/接受人=被邀请者，成员关系只能写入邀请所属 Team。

## 领域总览与后续规划

身份、审计与团队为已合并基线；项目/任务的特性分支 Schema 见下方 Phase 03 章节，尚未合入 develop。成果、贡献与奖金仍为后续设计，此处保留领域总览。

| 领域 | 表 | 关键约束 |
| --- | --- | --- |
| 身份 | `users`、`sessions`、`platform_roles` | 见 Phase 02 Schema；`users.system_root` 唯一真值 + ROOT 唯一部分索引 |
| 审计 | `audit_events` | 见 Phase 02 Schema；追加写、同事务、禁敏感载荷 |
| 团队 | `teams`、`team_members`、`team_invitations` | 见 Phase 02 Schema；`(team_id,user_id)` 主键；角色枚举；最后 owner 触发器 |
| 项目 | `projects`、`project_members`、`project_milestones` | `team_id` 外键；负责人必须是团队成员 |
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

## Phase 03 设计增补（2026-11-17）

数据库结构、复合外键、锁与历史策略见 [Phase 03 设计说明](../development/phase-03-design.md)。本次任务规则优先于前文历史规划；Phase 02 已合并于 develop `c3fb029`。实现验证另见阶段报告。

## Phase 03 Schema（特性分支开发与验证）

新增 `00004_projects.sql`、`00005_tasks.sql`，版本 3 → 5；00001–00003 原样保留。

| 表 | 约束与索引 |
| --- | --- |
| projects | UUID；UNIQUE(team_id,id)；负责人复合 FK→team_members；active/archived 一致性；version；计划先后；Team/创建时间、负责人索引 |
| project_members | PK(team_id,project_id,user_id)；复合项目及 Team 成员 FK；active/removed；用户/状态索引；lead 角色由 projects.lead_id 唯一推导，避免角色与负责人列不一致 |
| project_milestones | UUID；复合项目 FK；名称/说明/计划时间/version；项目索引 |
| tasks | UUID；UNIQUE(team_id,project_id,id)；复合项目/父任务/项目成员 FK；depth 1–3；六状态枚举；actual 0–99；weight 1–1000000；取消原因；version；项目树/负责人索引 |
| task_collaborators | 复合主键与任务/项目成员 FK；用户索引 |
| task_dependencies | UUID；同项目复合端点 FK；非自环/唯一；忽略 actor/time/reason 一致性；反向依赖索引 |
| task_progress_history | bigint；复合任务 FK；actor、old/new 状态/进度/权重、原因、变更前后项目 bps、request_id、时间；追加式触发器；任务/id 倒序索引 |
| project_progress_snapshots | bigint；复合任务 FK；actor、事件、精确 numeric 分子/分母、bps、有效叶子数、request_id、时间；追加式触发器；项目/id 倒序索引 |

沿用 FK RESTRICT。历史两表复用 audit 的禁止 UPDATE/DELETE 函数，不复制追加写基础设施。task_progress_history 同时保存执行与权重事件，避免另建重复的权重历史表。project_milestones 使用明确模块前缀。

跨行规则由 Team 行锁 + Service 校验承担；延迟结构触发器验证最终父子 depth 差值，严格递增的深度同时排除环。执行触发器拒绝未接入 Phase 04 的状态，校验 active owner；退出保护触发器要求项目负责人先交接，Team 移除后失效项目成员资格。普通 CHECK 不用于声称保证 active 成员或递归无环。历史时间及新业务更新时间使用 clock_timestamp() 记录锁后操作时刻，业务可见性在事务提交时生效。
