# 普通应用连接与迁移连接

Phase 05 M3 在生产 API 启动时只读检查数据库角色。迁移、初始化与备份由独立运维连接执行，普通 API 不能使用数据库/schema/table/function owner 或超级用户的凭据。检查拒绝高权限角色及其可切换的成员资格、CREATEDB/CREATEROLE/REPLICATION/BYPASSRLS、当前数据库 CREATE、业务 schema CREATE、schema/表/函数所有权、TRUNCATE 以及 audit_events 的 UPDATE/DELETE/TRUNCATE。同时拒绝 SET session_replication_role 的授权，防止通过 replica 模式绕过触发器。角色检查不会变更任何 ACL、数据库或运行容器。

当前开发 Compose 的 POSTGRES_USER 是开发初始化/迁移角色；此配置不等同于生产应用最小权限。M3 没有升级开发数据库、修改其角色或重启其服务。生产将普通 API 切换到受限凭据前，须由运维在目标 Nexora 专用数据库审核授权；迁移 CLI 使用单独受保护的凭据。无需将密码写入 SQL、仓库或日志。

以下是 **v7–v8 专用数据库** 的审核清单，不由应用自动执行；替换 schema/app_role/database 为实际运维选择，不要向其他项目数据库应用。角色应独立 LOGIN、无其他角色成员资格、非数据库 owner，密码由受控凭据管理渠道设置。不要把初始 owner 角色降权来兼作应用角色。应用 v8 迁移后必须重新执行一次 SELECT 授权，否则只读奖金接口会因缺少新表权限返回 500。

```sql
-- psql variables: schema, app_role, database. Run as the migration owner.
-- Role provisioning and credentials are managed outside this checked-in file.
REVOKE CREATE ON DATABASE :"database" FROM PUBLIC;
REVOKE CREATE ON SCHEMA :"schema" FROM PUBLIC;
GRANT CONNECT ON DATABASE :"database" TO :"app_role";
GRANT USAGE ON SCHEMA :"schema" TO :"app_role";
GRANT SELECT ON ALL TABLES IN SCHEMA :"schema" TO :"app_role";
-- v8 Phase06 bonus read models remain SELECT-only in M3. No INSERT/UPDATE/DELETE
-- is granted until the approved write workflow and its audited migration exist.
GRANT SELECT ON bonus_rounds,bonus_round_versions,bonus_round_sources,
 bonus_eligibility_decisions,bonus_allocation_lines,bonus_adjustments,
 bonus_role_grants,bonus_confirmations,bonus_events,bonus_member_totals
 TO :"app_role";
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA :"schema" TO :"app_role";
-- Resolve names in the reviewed schema for the remaining statements.
SET search_path TO :"schema";
GRANT INSERT ON users,sessions,platform_roles,teams,team_members,team_invitations,
 projects,project_members,project_milestones,project_reviewers,tasks,
 task_collaborators,task_dependencies,task_progress_history,project_progress_snapshots,
 task_submissions,task_submission_versions,task_reviews,submission_attachments,audit_events,
 contribution_appraisers,contribution_cases,contribution_case_versions,contribution_shares,
 contribution_share_adjustments,contribution_confirmations,contribution_conflicts,
 contribution_disputes,contribution_dispute_resolutions,contribution_case_events,
 contribution_notifications,contribution_attestations TO :"app_role";
GRANT UPDATE ON sessions,teams,team_members,team_invitations,projects,project_members,
 project_milestones,project_reviewers,tasks,submission_attachments,
 contribution_appraisers,contribution_cases,contribution_case_versions,contribution_shares
 TO :"app_role";
-- Phase 03's dependency-ignore operation updates only these three fields.
-- Column-scoped UPDATE avoids unnecessary permission to rewrite dependency identity.
GRANT UPDATE (ignored_by,ignored_at,ignore_reason) ON task_dependencies TO :"app_role";
GRANT DELETE ON platform_roles,task_collaborators,task_dependencies,contribution_shares
 TO :"app_role";
```

v7 函数为 SECURITY INVOKER，触发器继承当前调用权限，因此业务事件、份额调整和审计需 INSERT；SELECT FOR UPDATE/SHARE 也需要对应表 UPDATE 资格。没有向应用授予 DDL、事实表 UPDATE/DELETE、goose_db_version 写入或禁用触发器的权限。默认/继承 ACL 可能扩大权限，部署应检查最终有效权限并运行生产启动检查，不能仅确认 GRANT 语句已执行。

角色检查不替代 Service RBAC、Team/Project 作用域或会话认证，也不声称提供 RLS。受限应用凭据属于可信后端，不能分发给普通用户。后续新增迁移需单独复核新表/函数授权，禁止依靠宽泛的自动未来 DDL 权限。
