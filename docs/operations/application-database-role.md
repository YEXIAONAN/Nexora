# 普通应用连接与迁移连接

Phase 05 M3 在生产 API 启动时只读检查数据库角色。迁移、初始化与备份由独立运维连接执行，普通 API 不能使用数据库/schema/table/function owner 或超级用户的凭据。检查拒绝高权限角色及其可切换的成员资格、CREATEDB/CREATEROLE/REPLICATION/BYPASSRLS、当前数据库 CREATE、业务 schema CREATE、schema/表/函数所有权、TRUNCATE 以及 audit_events 的 UPDATE/DELETE/TRUNCATE。同时拒绝 SET session_replication_role 的授权，防止通过 replica 模式绕过触发器。存在 v11 时还必须有 bonus_workflow_command / bonus_workflow_workspace 的 EXECUTE 和 bonus_payments / bonus_commands 的 SELECT；拒绝任意奖金表的直接写入权限（含列级权限、REFERENCES/TRIGGER）以及其他 bonus_ 触发器/辅助函数的有效 EXECUTE 权限。角色检查不会变更任何 ACL、数据库或运行容器。

当前开发 Compose 的 POSTGRES_USER 是开发初始化/迁移角色；此配置不等同于生产应用最小权限。M3 没有升级开发数据库、修改其角色或重启其服务。生产将普通 API 切换到受限凭据前，须由运维在目标 Nexora 专用数据库审核授权；迁移 CLI 使用单独受保护的凭据。无需将密码写入 SQL、仓库或日志。

以下是 **v9–v11 专用数据库** 的审核清单，不由应用自动执行；替换 schema/app_role/database 为实际运维选择，不要向其他项目数据库应用。角色应独立 LOGIN、无其他角色成员资格、非数据库 owner，密码由受控凭据管理渠道设置。不要把初始 owner 角色降权来兼作应用角色。API 启动与就绪检查最低要求 v9（Phase07 进展上报），当前版本 v11（奖金工作流）；v8 不再可用于接收流量。应用 v9 迁移后须补授 task_progress_reports 的 SELECT/INSERT；v10 新增奖金基础表，通用 SELECT 授权覆盖已存在的新表。只有 v11 挂载奖金读写路由，v9/v10 均不挂载。升级至 v11 后须在通用授权之后执行下方 v11 configure 步骤，补齐新表/入口函数权限并收回直接写入和辅助函数权限，否则生产角色检查拒绝启动。

```sql
-- psql variables: schema, app_role, database. Run as the migration owner.
-- Role provisioning and credentials are managed outside this checked-in file.
REVOKE CREATE ON DATABASE :"database" FROM PUBLIC;
REVOKE CREATE ON SCHEMA :"schema" FROM PUBLIC;
GRANT CONNECT ON DATABASE :"database" TO :"app_role";
GRANT USAGE ON SCHEMA :"schema" TO :"app_role";
GRANT SELECT ON ALL TABLES IN SCHEMA :"schema" TO :"app_role";
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA :"schema" TO :"app_role";
-- Resolve names in the reviewed schema for the remaining statements.
SET search_path TO :"schema";
-- v9 Phase07 progress reports are append-only execution evidence.
GRANT INSERT ON task_progress_reports TO :"app_role";
GRANT INSERT ON users,sessions,platform_roles,teams,team_members,team_invitations,
 projects,project_members,project_milestones,project_reviewers,tasks,
 task_collaborators,task_dependencies,task_progress_history,project_progress_snapshots,
 task_submissions,task_submission_versions,task_reviews,submission_attachments,audit_events,
 contribution_appraisers,contribution_cases,contribution_case_versions,contribution_shares,
 contribution_share_adjustments,contribution_confirmations,contribution_conflicts,
 contribution_disputes,contribution_dispute_resolutions,contribution_case_events,
 contribution_notifications,contribution_attestations TO :"app_role";
GRANT UPDATE ON sessions,teams,team_members,team_invitations,projects,
 project_milestones,project_reviewers,tasks,submission_attachments,
 contribution_appraisers,contribution_cases,contribution_case_versions,contribution_shares
 TO :"app_role";
-- Remove an earlier whole-table UPDATE grant; membership epochs are trigger-managed.
REVOKE UPDATE ON project_members FROM :"app_role";
GRANT UPDATE (status,updated_at) ON project_members TO :"app_role";
-- Phase 03's dependency-ignore operation updates only these three fields.
-- Column-scoped UPDATE avoids unnecessary permission to rewrite dependency identity.
GRANT UPDATE (ignored_by,ignored_at,ignore_reason) ON task_dependencies TO :"app_role";
GRANT DELETE ON platform_roles,task_collaborators,task_dependencies,contribution_shares
 TO :"app_role";
```

仅在已迁移到 **v11** 后，由 **migration owner** 在上述通用授权之后、相同 search_path 的会话中执行以下步骤（v9/v10 跳过，函数尚不存在）：

```sql
-- v11: SELECT on bonus tables, EXECUTE only on command/workspace entry points.
-- Revoke direct bonus DML and all bonus trigger/helper EXECUTE from this role.
SELECT bonus_configure_application_role(:'app_role'::name);
```

此步骤也适用于已有生产角色升级到 v11；迁移本身不会猜测或修改应用角色。**不可在 configure 之后用全量 EXECUTE 授权修复接口权限**，这会重新开放触发器/辅助函数并被角色检查拒绝；如确需重跑通用授权，必须由 migration owner 再执行 configure 并检查最终有效权限。继承或 PUBLIC 的额外授权须单独撤销，不能依靠针对应用角色的 REVOKE 消除。

v7 函数为 SECURITY INVOKER，触发器继承当前调用权限，因此业务事件、份额调整和审计需 INSERT；SELECT FOR UPDATE/SHARE 也需要对应表 UPDATE 资格。没有向应用授予 DDL、事实表 UPDATE/DELETE、goose_db_version 写入或禁用触发器的权限。默认/继承 ACL 可能扩大权限，部署应检查最终有效权限并运行生产启动检查，不能仅确认 GRANT 语句已执行。

角色检查不替代 Service RBAC、Team/Project 作用域或会话认证，也不声称提供 RLS。SQL 的 session 参数由通过 cookie 中间件认证的服务传入；数据库账号属于可信后端，不可交给用户，且具有 sessions 的 SELECT/INSERT/UPDATE 权限，凭据泄露可伪造身份，不能声称抵御数据库凭据被完全攻陷；独立核定的 HTTP 测试必须使用真正的登录 cookie。后续新增迁移需单独复核新表/函数授权，禁止依靠宽泛的自动未来 DDL 权限。
