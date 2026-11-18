package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUnsafeApplicationRole = errors.New("production API requires a non-owner application role without schema DDL, privileged role membership, truncate or audit mutation privileges; use a separate migration connection")

// ValidateApplicationRole is read-only. It never changes roles, ACLs or the
// schema. Production startup must not share the migration owner's credential.
// Application RBAC remains the service's responsibility; this is not RLS.
func ValidateApplicationRole(ctx context.Context, pool *pgxpool.Pool) error {
	var unsafe bool
	err := pool.QueryRow(ctx, `SELECT
 current_schema() IS NULL
 OR EXISTS(SELECT 1 FROM pg_roles r WHERE pg_has_role(current_user,r.oid,'MEMBER') AND (r.rolsuper OR r.rolcreaterole OR r.rolcreatedb OR r.rolreplication OR r.rolbypassrls OR r.rolname IN ('pg_read_all_data','pg_write_all_data','pg_read_server_files','pg_write_server_files','pg_execute_server_program')))
 OR has_database_privilege(current_user,current_database(),'CREATE')
 OR has_parameter_privilege(current_user,'session_replication_role','SET')
 OR EXISTS(SELECT 1 FROM pg_namespace n WHERE n.nspname !~ '^pg_' AND n.nspname<>'information_schema' AND (has_schema_privilege(current_user,n.oid,'CREATE') OR pg_has_role(current_user,n.nspowner,'MEMBER')))
 OR EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname !~ '^pg_' AND n.nspname<>'information_schema' AND (pg_has_role(current_user,c.relowner,'MEMBER') OR c.relkind IN ('r','p') AND has_table_privilege(current_user,c.oid,'TRUNCATE')))
 OR EXISTS(SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname !~ '^pg_' AND n.nspname<>'information_schema' AND pg_has_role(current_user,p.proowner,'MEMBER'))
 OR has_table_privilege(current_user,'audit_events','UPDATE,DELETE,TRUNCATE')`).Scan(&unsafe)
	if err != nil {
		return fmt.Errorf("check application database role: %w", SafeError(err))
	}
	if unsafe {
		return ErrUnsafeApplicationRole
	}
	return nil
}
