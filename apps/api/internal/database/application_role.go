package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUnsafeApplicationRole = errors.New("production API requires a non-owner application role without schema DDL, privileged role membership, truncate or audit mutation privileges, and with configured v11 bonus workflow privileges when present; use a separate migration connection")

// ValidateApplicationRole is read-only. It never changes roles, ACLs or the
// schema. Production startup must not share the migration owner's credential.
// Application RBAC remains the service's responsibility; this is not RLS.
// v9/v10 do not have the v11 entry points or its mandatory restricted ACLs.
func ValidateApplicationRole(ctx context.Context, pool *pgxpool.Pool) error {
	var unsafe bool
	err := pool.QueryRow(ctx, `WITH bonus_api AS (
 SELECT to_regprocedure(quote_ident(current_schema())||'.bonus_configure_application_role(name)') AS configure,
 to_regprocedure(quote_ident(current_schema())||'.bonus_workflow_command(uuid,uuid,uuid,uuid,uuid,text,jsonb)') AS command,
 to_regprocedure(quote_ident(current_schema())||'.bonus_workflow_workspace(uuid,uuid,uuid,uuid,uuid)') AS workspace
 ) SELECT
 current_schema() IS NULL
 OR EXISTS(SELECT 1 FROM pg_roles r WHERE pg_has_role(current_user,r.oid,'MEMBER') AND (r.rolsuper OR r.rolcreaterole OR r.rolcreatedb OR r.rolreplication OR r.rolbypassrls OR r.rolname IN ('pg_read_all_data','pg_write_all_data','pg_read_server_files','pg_write_server_files','pg_execute_server_program')))
 OR has_database_privilege(current_user,current_database(),'CREATE')
 OR has_parameter_privilege(current_user,'session_replication_role','SET')
 OR EXISTS(SELECT 1 FROM pg_namespace n WHERE n.nspname !~ '^pg_' AND n.nspname<>'information_schema' AND (has_schema_privilege(current_user,n.oid,'CREATE') OR pg_has_role(current_user,n.nspowner,'MEMBER')))
 OR EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname !~ '^pg_' AND n.nspname<>'information_schema' AND (pg_has_role(current_user,c.relowner,'MEMBER') OR c.relkind IN ('r','p') AND has_table_privilege(current_user,c.oid,'TRUNCATE')))
 OR EXISTS(SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname !~ '^pg_' AND n.nspname<>'information_schema' AND pg_has_role(current_user,p.proowner,'MEMBER'))
 OR has_table_privilege(current_user,'audit_events','UPDATE,DELETE,TRUNCATE')
 OR EXISTS(SELECT 1 FROM bonus_api b WHERE b.configure IS NOT NULL AND (
 NOT COALESCE(has_function_privilege(current_user,b.command,'EXECUTE'),false)
 OR NOT COALESCE(has_function_privilege(current_user,b.workspace,'EXECUTE'),false)
 OR EXISTS(SELECT 1 FROM (VALUES ('bonus_payments'),('bonus_commands')) AS required(name)
 WHERE NOT COALESCE(has_table_privilege(current_user,to_regclass(quote_ident(current_schema())||'.'||required.name),'SELECT'),false))
 OR EXISTS(SELECT 1 FROM pg_class c WHERE c.relnamespace=current_schema()::regnamespace
 AND left(c.relname,6)='bonus_' AND c.relkind IN ('r','p','v','m','f') AND (
 has_table_privilege(current_user,c.oid,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')
 OR has_any_column_privilege(current_user,c.oid,'INSERT,UPDATE,REFERENCES')))
 OR EXISTS(SELECT 1 FROM pg_proc p WHERE p.pronamespace=current_schema()::regnamespace
 AND left(p.proname,6)='bonus_' AND p.oid IS DISTINCT FROM b.command AND p.oid IS DISTINCT FROM b.workspace
 AND has_function_privilege(current_user,p.oid,'EXECUTE'))
 ))`).Scan(&unsafe)
	if err != nil {
		return fmt.Errorf("check application database role: %w", SafeError(err))
	}
	if unsafe {
		return ErrUnsafeApplicationRole
	}
	return nil
}
