# Phase 02 验收报告：认证、团队与权限

- 日期：2026-11-17
- 范围：仅 Phase 02（身份认证、会话、ROOT 引导、多团队与独立角色、团队权限、审计、最小前端闭环）。不包含项目/任务/状态机/成果/验收/贡献/奖金等业务。
- 批准依据：产品负责人 2026-11-17 选择方案 B；V1.0 PRD 生效；ADR 0004 Accepted；ADR 0006 Accepted。
- 分支：`feature/phase02-auth-tenancy`（基于 develop `1cfa22d`），PR → develop，不自动合并。
- 结论：Phase 02 后端与最小前端闭环代码已开发并通过真实验证（已验证）；身份 1–7 与团队 8–18 安全场景由真实 PostgreSQL 集成测试覆盖，第 19 项“批量越权”因 Phase 02 无批量接口仅验证路由不存在；11 项浏览器 E2E 在一次性真实栈通过；本地全部门禁绿。代码位于特性分支，PR #2 待合并、尚未合入 develop，按 PRD 约定属“已开发并验证”而非“已实现/正式集成完成”。Phase 02 完成不代表 V1.0 交付完成。

> 状态用语遵循 PRD 约定：**已开发**指代码在特性分支完成、尚未合入 develop；**已实现**指代码已合入 develop（或 main）并通过测试；**已验证**指有真实执行证据。本报告所述 Phase 02 成果均为“代码已开发 + 测试已通过（已验证）”，PR #2 尚待合并、未合入 develop，故不标注“已实现”或“正式集成完成”。本报告所有结果均为本机实际执行，未使用模拟数据或伪造输出。

## A. 实际开发内容（按模块）

| 模块 | 状态 | 路径 | 验证依据 |
| --- | --- | --- | --- |
| 身份认证 auth | 已开发并验证 | `apps/api/internal/auth/` | `service_integration_test.go`（16 测试）+ `internal/httpapi/auth_integration_test.go`（9 测试），真实 PG + `-race` |
| ROOT 引导 | 已开发并验证 | `apps/api/cmd/bootstrap/` | `TestBootstrap*`、`TestWaitingUsernameDoesNotGrantRoot` |
| 团队与 RBAC teams | 已开发并验证 | `apps/api/internal/teams/` | `service_integration_test.go`（19 测试）+ `internal/httpapi/teams_integration_test.go`（9 测试） |
| 审计 audit | 已开发并验证 | `apps/api/internal/audit/` | `TestPermissionChangesAreAudited`、`TestAuditEventsAreAppendOnly`、`TestFailedPermissionChangeLeavesNoAudit` |
| 迁移 | 已开发并验证 | `apps/api/internal/database/migrations/00002_auth.sql`、`00003_teams.sql` | 全新/升级/回滚一次性实例验证（见 B） |
| 前端 API 客户端与 store | 已开发并验证 | `apps/web/src/api/{client,auth,teams}.ts`、`apps/web/src/stores/{auth,teams}.ts` | Vitest：client 8、auth 7、teams 7 |
| 前端视图与布局 | 已开发并验证 | `apps/web/src/views/{Login,Teams,TeamMembers,Invitations}View.vue`、`components/layout/*` | Vitest：LoginView 5、TeamsView 3、TeamMembersView 9（切换竞态）；浏览器 E2E 11 |
| 路由守卫与会话失效 | 已开发并验证 | `apps/web/src/router/index.ts`、`main.ts`、`App.vue` | E2E 未认证重定向、撤销会话回登录页 |
| OpenAPI 契约 | 已开发并验证 | `docs/api/openapi.yaml` → `apps/web/src/api/schema.d.ts` | `pnpm api:check` 一致 |
| 浏览器 E2E 基础设施 | 已开发并验证 | `apps/web/e2e/phase02.spec.ts`、`playwright.config.ts`、`infra/scripts/e2e.sh` | `make e2e` 11/11 通过 |

部分/未完成（保留设计，不声称已实现）：团队信息编辑/归档、批量成员操作、密码自助修改/重置、敏感操作重新认证、ROOT 恢复流程、邮件投递、审计日志只读页。项目完整业务及后续阶段见 G。

## B. 数据库变化

迁移只追加，不修改已发布的 `00001_baseline.sql`。版本推进至 `3`。

### 00002_auth.sql

- `users`：`id uuid PK`、`username text UNIQUE`（`^[a-z0-9_-]{3,32}$`）、`password_hash`（非空）、`system_root boolean`、时间戳。`users_single_system_root` 部分唯一索引（`WHERE system_root`）保证全库恰一个 ROOT，并发引导无法创建第二个。
- `sessions`：`id uuid PK`、`user_id FK→users RESTRICT`、`token_hash text UNIQUE`（`^[0-9a-f]{64}$`，仅存 SHA-256 摘要）、`expires_at`、`revoked_at`；约束 `expires_at > created_at`、撤销时间一致性。索引 `user_id`、`expires_at`。
- `platform_roles`：`PK (user_id, role)`，`role IN ('platform_admin')`，`granted_by FK→users`。ROOT 本身不入此表。
- `audit_events`：`id bigint identity PK`、`actor_id`、`action`、`target_type/target_id`、`team_id`、`request_id`、`details jsonb`、`created_at`。追加写：`BEFORE UPDATE OR DELETE` 触发器禁止修改/删除。索引 `(team_id,created_at)`、`(actor_id,created_at)`、`(target_type,target_id)`、`created_at`。

### 00003_teams.sql

- `teams`：`id uuid PK`、`name`（1–64）、`description`（≤500）、`created_by FK→users`、`archived_at`。
- `team_members`：`PK (team_id, user_id)`（复合键，未来复合 FK 的目标），`role IN ('owner','admin','member')`、`status IN ('active','removed')`、`removed_at` 与 status 一致性约束。索引 `(user_id,status)`、部分索引 `(team_id) WHERE active AND owner`。`team_members_last_owner_guard` 触发器在 UPDATE role/status 或 DELETE 时保证至少一名 active owner（配合 Service 对 teams 行 `FOR UPDATE` 串行化）。
- `team_invitations`：`id uuid PK`、`team_id/invited_user_id/invited_by FK→users/teams`、`role IN ('admin','member')`、`status IN ('pending','accepted','declined','cancelled')`、`expires_at > created_at`、响应一致性。部分唯一索引 `(team_id,invited_user_id) WHERE pending` 防重复 pending；索引 `(invited_user_id,status)`、`(team_id,status)`。
- `audit_events.team_id` 追加外键 `→teams RESTRICT`。

### 跨 Team 完整性

所有团队资源以 `teams.id` 为边界；`team_members` 复合主键 `(team_id,user_id)` 为后续业务表复合外键防跨团队错误挂接奠定基础。Repository 查询强制带 `team_id` 条件；授权在 Service 层按请求实时判定，不在会话缓存成员关系。

### 迁移验证（一次性实例，真实执行）

在一次性容器 `nexora-devtest-pg`（loopback `127.0.0.1:5433`）的专用临时库 `nexora_mig_verify` 上，用 `nexora-migrate` 二进制逐步验证，退出即删库：

| 步骤 | 操作 | 期望版本 | 结果 |
| --- | --- | --- | --- |
| 全新空库 | `up` | 3 | OK |
| 完整回滚 | `down -steps 3` | 0 | OK |
| 回滚后重新应用 | `up` | 3 | OK |
| 回滚到 v1 基线 | `down -steps 2` | 1 | OK |
| v1 升级 | `up` | 3 | OK |
| 逐级回滚 | `down -steps 1` ×3 | 2→1→0 | OK |

集成测试 `TestMigrationsApplyIdempotently` 另验证幂等（二次 `up` 无操作）与随机 schema 隔离。运行中的开发库（5432）未被触碰，未在其上自动执行新迁移。

## C. API 清单与契约一致性

统一信封：成功 `{"data": ...}`，失败 `{"error": {code, message, fields?, request_id}}`；响应携带 `X-Request-Id`。契约源 `docs/api/openapi.yaml`（OpenAPI 3.1），生成类型 `apps/web/src/api/schema.d.ts`，`pnpm api:check` 内存比对一致（不写源码）。

| 分组 | 端点 |
| --- | --- |
| 诊断 | `GET /healthz`、`GET /readyz`、`GET /api/v1/meta`（含 `root_initialized`） |
| 认证 | `POST /api/v1/auth/login`、`GET /api/v1/auth/me`、`DELETE /api/v1/auth/session` |
| 平台管理（ROOT/平台管理员） | `POST /api/v1/admin/users`、`DELETE /api/v1/admin/users/{userID}/sessions`、`POST /api/v1/admin/platform-roles`、`DELETE /api/v1/admin/platform-roles/{userID}`、`GET /api/v1/admin/teams` |
| 团队 | `GET/POST /api/v1/teams`、`GET /api/v1/teams/{teamID}`、`GET /api/v1/teams/{teamID}/permissions` |
| 成员 | `GET /api/v1/teams/{teamID}/members`、`PATCH/DELETE /api/v1/teams/{teamID}/members/{userID}` |
| 邀请 | `GET/POST /api/v1/teams/{teamID}/invitations`、`DELETE /api/v1/teams/{teamID}/invitations/{invitationID}`、`GET /api/v1/invitations`、`POST /api/v1/invitations/{invitationID}/accept`、`POST /api/v1/invitations/{invitationID}/decline` |

共 24 个 operation。响应映射：未认证 401；无团队访问权 404（含 ROOT，避免泄漏存在性）；权限不足 403；状态冲突 409；参数非法 400/422。`POST /api/v1/teams` 的 201 响应携带数据库真实 `created_at/updated_at` 与创建者 `my_role=owner`（M4 修复）。

## D. 安全测试结果（真实 PostgreSQL，`-race`）

全部经 `internal/testdb` 护栏：`NEXORA_ENV=test`、`NEXORA_TEST_DB_DISPOSABLE=1`、loopback、`nexora_test*` 库名、随机 `nexora_it_` schema 自动清理。未连接运行中的开发库、生产库或其他项目数据库。`go test -race -count=1 -tags=integration ./...` 全部包通过。

### 身份 1–7

| # | 场景 | 覆盖测试 | 结果 |
| --- | --- | --- | --- |
| 1 | 无默认 ROOT 密码 | `TestBootstrapRequiresExplicitStrongPassword`、`TestNoPublicRegistrationRoute` | 通过 |
| 2 | 普通用户不能改 system_root | `TestCreateUserAuthorization`、`TestWaitingUsernameDoesNotGrantRoot` | 通过 |
| 3 | 重复引导拒绝 | `TestBootstrapRefusesRepeat` | 通过 |
| 4 | 并发引导单 ROOT | `TestBootstrapConcurrentCreatesExactlyOneRoot` | 通过 |
| 5 | 撤销/过期会话失效 | `TestLogoutRevokesSession`、`TestExpiredSessionCannotAuthenticate`、`TestAdminRevokeUserSessions`、`TestMeLogoutFlow` | 通过 |
| 6 | 登录限流 | `TestLoginRateLimit`、`TestLoginRateLimitHTTP` | 通过 |
| 7 | CSRF 防护 | `TestTeamRoutesRequireSessionAndCSRF`、`TestProtectedEndpointRequiresSession`；登录入口经真实 HTTP 路由验证（PR #2 整改新增）：`TestLoginRequiresCSRFHeader`、`TestLoginWithCSRFHeaderSucceeds`、`TestLoginRejectsNonJSONContentType`、`TestLoginCrossSiteFormSpoofIsBlocked` | 通过 |

### Team 8–19

| # | 场景 | 覆盖测试 | 结果 |
| --- | --- | --- | --- |
| 8 | 用户加入 A+B | `TestTwoTeamsIndependentRoles`、`TestInvitationLifecycleRules` | 通过 |
| 9 | A owner / B member 权限不同 | `TestTwoTeamsIndependentRoles` | 通过 |
| 10 | A 角色不能改 B | `TestRoleInOneTeamGrantsNoAuthorityInAnother` | 通过 |
| 11 | 改 URL team_id 不可跨团队 | `TestURLTeamIDTamperingIsNotFound`、`TestNonMemberAndRootCannotReadTeam` | 通过 |
| 12 | 改请求体 team_id 不绕过 | `TestBodyTeamIDCannotOverridePath`、`TestCrossTeamInvitationReferenceIsRejected` | 通过 |
| 13 | 未加入无法读取 | `TestNonMemberAndRootCannotReadTeam` | 通过 |
| 14 | 移除 A 后原会话立即失 A、仍可 B | `TestRemovalTakesEffectImmediately`、`TestRemovedMemberCannotActEvenAsFormerOwner` | 通过 |
| 15 | 团队管理员不能授 ROOT/平台管理员 | `TestTeamOwnerIsNotPlatformAdmin`、`TestPlatformRoleManagementIsRootOnly`、`TestAdminCannotGrantOwnerOrTouchOwners` | 通过 |
| 16 | 最后 owner 不得移除 | `TestLastOwnerProtection`、`TestSelfLeaveByMemberAndAdmin` | 通过 |
| 17 | 并发不破坏约束 | `TestConcurrentMutationsPreserveInvariants` | 通过 |
| 18 | 权限变更产生审计 | `TestPermissionChangesAreAudited`、`TestFailedPermissionChangeLeavesNoAudit`、`TestAuditEventsAreAppendOnly` | 通过 |
| 19 | 批量越权整体失败 | `TestNoBatchMemberRoutes`：仅证明 Phase 02 不存在批量成员路由（返回 404/405）；邀请/成员单操作的事务原子性由相应集成测试覆盖 | 不适用（Phase 02 无批量接口，未执行真实批量越权测试；待批量接口出现时须补“越权整体失败”专项测试） |

补充验证：waiting 同名无 ROOT（`TestWaitingUsernameDoesNotGrantRoot`）；用户枚举（`TestLoginHidesAccountExistence`）；会话固定（`TestLoginAlwaysIssuesFreshToken`）；令牌仅摘要（`TestLoginStoresOnlyTokenDigest`）；Cookie 属性与生产强制 Secure（`TestLoginCookieAttributes`、`TestProductionCookieForcesSecure`）；过期/重复 pending 邀请与 TTL（`TestInvitationLifecycleRules`、`TestInvitationDefaultAndCustomTTL`）；ROOT 平台可见性（`TestPlatformAdminListsAllTeams`、`TestAdminTeamsRequiresPlatformRole`）；拒绝审计不死锁（`TestAuditDenialDoesNotBlockRefusal`）；团队名按团队非全局（`TestTeamNamesAreScopedPerTeamNotGlobal`）。

### Argon2id 参数基准（ADR 0006）

参数：`argon2id v=19, t=3, m=65536 KiB（64 MiB）, p=4, key=32B, salt=16B`。本机实测 `BenchmarkHashPassword-12`：约 **79.8 ms/次**（`79811334 ns/op`，`-benchtime=50x`）。单次哈希成本足以抬高离线爆破代价，登录接口另叠加 IP+用户名限流。

## E. 前端真实 API 联调

- 所有业务数据经后端真实 API，无 Mock 伪装；长期认证令牌不进 localStorage（仅持久化“当前 Team 偏好”这类非凭据数据，E2E 断言 localStorage 无 token/session/password 键）。
- 多 Team 由 store + 顶栏选择器表示并正确切换，无硬编码 ZERO_R Team ID；团队角色按服务端权限矩阵渲染管理控件。
- 闭环：未认证重定向、ROOT 初始化提示、登录/退出、团队创建/列表/切换、成员角色变更/移除、邀请发出/接受/拒绝、无权限与过期状态、移除后原会话立即失去团队访问（404）、最后 owner 退出保护（409）。
- Vitest 10 文件 / 57 用例通过（含 TeamMembersView 切换竞态 9 用例）；浏览器 E2E 11/11 通过（系统 Chrome，`channel:'chrome'`，无需下载浏览器）。

### 浏览器 E2E（一次性真实栈）

`make e2e`（`infra/scripts/e2e.sh`）仅连接一次性容器 `nexora-devtest-pg`（loopback 5433），重建专用 `nexora_e2e` 库并在退出删除；psql 经 `docker exec` 在容器内执行；ROOT 密码每次随机生成仅经环境变量注入、不落盘不打印；API 监听 `127.0.0.1:8081`、Vite `127.0.0.1:5173`；trap 清理进程与库。不触碰开发栈（5432/8080/3000）、其他项目容器或卷。

11 项串行用例全部通过（27.8s）：未认证重定向；ROOT 登录并创建首个团队；经真实管理 API 创建账号；成员页邀请 alice；alice 收件箱接受；普通成员无管理控件；ROOT 晋升 alice 为 admin；移除 alice 后其访问立即终止；最后 owner 不能退出；顶栏多团队切换；撤销会话回登录页并提示。E2E 为本地门禁，CI 仍为四 job。

## F. Git 与 CI

- 分支 `feature/phase02-auth-tenancy`，基于 develop `1cfa22d`；`main` 保持 `36bddd6` 未修改，develop 未被本次整改修改。
- PR #2（→ develop）已创建并推送，**未合并**。本次针对 PR #2 Review 的两项阻断问题（登录 CSRF、Team 切换竞态）以追加普通提交方式整改：不 rebase 已发布历史、不 force push、无 AI 联合署名（Conventional Commits，作者 `waiting <yexiaonanair@outlook.com>`）。
- 本地等价门禁全部通过（见下表）。CI 四 job（Backend / Frontend / Secret scan / Containers）以 PR #2 最新 HEAD 的实际运行为准；最终提交 SHA、CI 结果与 Review Thread 处置见本次定向整改的最终汇报。整改与 CI 通过均不代表已合入 develop。

### 本地工程验证（真实执行）

| 检查项 | 命令 | 结果 |
| --- | --- | --- |
| Go 格式 | `gofmt -l apps/api/cmd apps/api/internal` | 无输出 |
| Go vet | `go vet ./...` | 通过 |
| Go 单元 | `go test ./...` | 全部通过 |
| Go 集成 + 竞态 | `go test -race -count=1 -tags=integration ./...` | 全部包通过（真实 PG17） |
| 前端 lint | `pnpm lint` | 通过 |
| 前端格式 | `pnpm format:check` | 通过 |
| 前端类型 | `pnpm typecheck`（vue-tsc） | 通过 |
| 前端单元 | `pnpm test` | 10 文件 / 57 用例通过 |
| 前端构建 | `pnpm build` | 成功，69 模块；主 JS 125.10 kB（gzip 48.86 kB） |
| 契约一致 | `pnpm api:check` | 一致 |
| 扫描器回归 | `python3 -m unittest ... test_secret_scan.py` | 8 测试通过 |
| 密钥扫描 | `infra/scripts/check-secrets.sh` | 通过 |
| 总门禁 | `make verify` | 退出码 0 |
| Compose 配置 | `docker compose --profile full config` | 通过 |
| 迁移全新/升级/回滚 | 一次性实例 `nexora-migrate` | 全部 OK（见 B） |
| 浏览器 E2E | `make e2e` | 11/11 通过 |

### 验证中发现并修复的问题

1. **创建团队响应缺真实时间戳与角色**：`InsertTeam` 原用 `Exec` 未取回 `created_at/updated_at`，`CreateTeam` 返回内存对象（零时间、无 `my_role`），导致 E2E 团队行渲染“—”与零日期。改为 `INSERT ... RETURNING` 扫描时间戳、`CreateTeam` 返回 `TeamSummary{Team, MyRole: owner}`，Handler 渲染 `my_role`；同步 OpenAPI 描述与生成类型，并补集成断言。
2. **密钥扫描误报 shell 变量展开**：`postgres-url-password` 规则把 E2E 脚本中的 `postgres://nexora:${PGPASSWORD}@...` 当作凭据，而 `postgres-password-assignment` 规则早已豁免纯变量展开。统一两条规则：纯 `${VAR}`（可带引号）视为运行时注入不予命中；字面口令、混合字面与变量、查询参数字面 `password/sslpassword` 仍失败。新增正反例回归测试证明不削弱真实凭据检测。此为误报修正，非降低标准。
3. **前端若干联调修正**：视图显式导入 `RouterLink` 以兼容无 router 插件的单测；成员页成功提示在 `loadAll` 重置之后设置；logout 服务端撤销失败时仍前端过期（best effort）。

### PR #2 Review 定向整改（本次，未合并）

针对 PR #2 代码审查的两项阻断问题定向修复，仅改动相关代码与回归测试，不扩大产品范围：

1. **登录 CSRF（阻断项一）**：原 `/api/v1/auth/login` 位于 CSRF 中间件之外，且 `httpx.DecodeBody` 不校验 Content-Type。跨站顶层表单可用 `enctype=text/plain` 构造合法 JSON 凭据体，使受害者浏览器被写入攻击者会话 Cookie（SameSite=Lax 不阻止顶层导航设 Cookie）。修复：将 `auth.CSRF` 上移至 `/api/v1` 路由级，覆盖未认证登录 POST（自定义头 `X-Nexora-CSRF` 表单无法伪造）；`DecodeBody` 用 `mime.ParseMediaType` 强制 `application/json`，拒绝 text/plain 等表单伪装。保持 HttpOnly/SameSite/Secure 与服务端会话不变，未引入宽松 CORS。新增 4 项经真实 HTTP 路由的集成测试；在原始 HEAD 上复现：无 CSRF 头登录、text/plain 表单伪装均返回 200 并建立会话（4 项中 3 项失败），修复后全部通过。
2. **Team 切换竞态（阻断项二）**：原 `TeamMembersView.loadAll()` 的旧异步响应可覆盖新 Team 状态，旧控件可误操作新 Team。修复：每次加载绑定显式 Team ID + 请求世代号（`loadGeneration`），`confirmedTeamId` 仅在全部响应落地后置位；`stale()` 同时校验世代与路由 ID，过期响应不得写状态或改 Team 偏好；`actionsEnabled` 在路由切换瞬间禁用旧控件；角色变更/移除/邀请/取消邀请均在点击时捕获已确认 Team ID 发送，并在应用结果前复核路由一致。新增 9 项确定性 Vitest（可控 deferred Promise，无 sleep/随机延迟）；在原始视图上复现 6 项失败，修复后 9/9 通过。

整改后完整回归见本地门禁与最终汇报；PR #2 仍待合并，未合入 develop。

## G. 进入 Phase 03 还缺什么

Phase 02 已交付身份与团队权限底座。Phase 03（项目、三级任务、状态机、依赖、进度聚合）开始前需：

1. **业务数据模型**：`projects`、三级 `tasks`、任务依赖与状态机表；所有团队业务表带 `team_id NOT NULL` 与复合外键 `(team_id, ...)` 防跨团队挂接（复用 `team_members` 复合键模式）。
2. **授权链扩展**：在团队角色之上引入项目角色（如适用），沿用 Service 层授权与 Repository team 约束；状态机非法转换、进度权重聚合需专项集成测试。
3. **状态机与进度规则**：非法转换 409、进度由任务权重聚合（不接受前端手工百分比）的领域逻辑与测试。
4. **前端业务视图**：项目/任务工作台，沿用真实 API 与多团队切换，不硬编码 Team ID。
5. **审计与契约**：新增业务操作同事务落审计；OpenAPI 扩展 → 生成类型 → 契约检查。
6. **遗留候选（非 Phase 03 阻塞）**：团队信息编辑/归档、批量成员操作（出现时补越权整体失败测试）、密码自助修改/重置、敏感操作重新认证、ROOT 恢复流程、审计日志只读页、邮件投递。

## 环境限制与未开发范围（如实记录）

- 网络：`github.com:443` 直连受限，推送经临时 DNS 解析与钥匙串凭据（仅环境变量复用，不打印、不写配置）；Go 模块经 `goproxy.cn`，校验仍由 `go.sum` 保证。
- 未执行真实生产部署、TLS 终止、压测与云环境演练；不列为已完成。
- 未开发项目完整业务、三级任务、状态机、成果验收、贡献核算、奖金分配、甘特图、高级统计、Nexora CLI、微服务拆分；保留 V2.0 原始需求为长期规划，不无痕覆盖。
- 一次性验证容器与临时库在验证后清理；运行中的开发栈与其他项目资源全程未触碰。