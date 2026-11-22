//go:build integration

package contributions_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/audit"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/auth"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/config"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/contributions"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpapi"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/system"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/tasks"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/teams"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/testdb"
)

func TestContributionApplicationRoleCannotBypassHistory(t *testing.T) {
	f := setup(t)
	if e := database.ValidateApplicationRole(f.ctx, f.db.Pool); !errors.Is(e, database.ErrUnsafeApplicationRole) {
		t.Fatalf("migration owner must be refused for production serving: %v", e)
	}
	role := "nexora_m3_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	var schema string
	must(t, f.db.Pool.QueryRow(f.ctx, `SELECT current_schema()`).Scan(&schema))
	qr, qs := pgx.Identifier{role}.Sanitize(), pgx.Identifier{schema}.Sanitize()
	// This role exists only in the explicitly disposable test server; its random
	// credential is never logged, included in source or put into a response.
	secret := uuid.NewString() + uuid.NewString()
	_, e := f.db.Pool.Exec(f.ctx, `CREATE ROLE `+qr+` LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD '`+secret+`'`)
	must(t, e)
	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, e := f.db.Pool.Exec(ctx, `REVOKE ALL PRIVILEGES ON PARAMETER session_replication_role FROM `+qr+`; REVOKE ALL ON ALL TABLES IN SCHEMA `+qs+` FROM `+qr+`; REVOKE ALL ON ALL FUNCTIONS IN SCHEMA `+qs+` FROM `+qr+`; REVOKE ALL ON SCHEMA `+qs+` FROM `+qr+`; DROP ROLE `+qr)
		if e != nil {
			t.Error("disposable role cleanup failed")
		}
	})
	_, e = f.db.Pool.Exec(f.ctx, `GRANT USAGE ON SCHEMA `+qs+` TO `+qr+`; GRANT SELECT ON ALL TABLES IN SCHEMA `+qs+` TO `+qr+`; GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA `+qs+` TO `+qr+`; GRANT INSERT ON `+qs+`.sessions,`+qs+`.audit_events,`+qs+`.contribution_appraisers,`+qs+`.contribution_cases,`+qs+`.contribution_case_versions,`+qs+`.contribution_shares,`+qs+`.contribution_share_adjustments,`+qs+`.contribution_confirmations,`+qs+`.contribution_conflicts,`+qs+`.contribution_disputes,`+qs+`.contribution_dispute_resolutions,`+qs+`.contribution_case_events,`+qs+`.contribution_notifications,`+qs+`.contribution_attestations TO `+qr+`; GRANT UPDATE ON `+qs+`.teams,`+qs+`.sessions,`+qs+`.tasks,`+qs+`.contribution_appraisers,`+qs+`.contribution_cases,`+qs+`.contribution_case_versions,`+qs+`.contribution_shares TO `+qr+`; GRANT DELETE ON `+qs+`.contribution_shares TO `+qr)
	must(t, e)
	// Apply the reviewed column-level GRANT from the production deployment guide
	// itself, so documentation drift breaks this real restricted-LOGIN test.
	doc, docErr := os.ReadFile("../../../../docs/operations/application-database-role.md")
	must(t, docErr)
	const documentedDependencyUpdate = `GRANT UPDATE (ignored_by,ignored_at,ignore_reason) ON task_dependencies TO :"app_role";`
	if !strings.Contains(string(doc), documentedDependencyUpdate) {
		t.Fatal("production application-role grants omit the Phase 03 dependency-ignore UPDATE")
	}
	grantSQL := strings.ReplaceAll(documentedDependencyUpdate, "task_dependencies", qs+".task_dependencies")
	grantSQL = strings.ReplaceAll(grantSQL, `:"app_role"`, qr)
	_, e = f.db.Pool.Exec(f.ctx, grantSQL)
	must(t, e)
	// Phase07 v9 progress reports require INSERT as well as SELECT. Keep the
	// reviewed grant append-only and sourced from the deployment guide.
	const documentedProgressInsert = `GRANT INSERT ON task_progress_reports TO :"app_role";`
	if !strings.Contains(string(doc), documentedProgressInsert) {
		t.Fatal("production application-role grants omit the Phase07 progress-report INSERT")
	}
	grantSQL = strings.ReplaceAll(documentedProgressInsert, "task_progress_reports", qs+".task_progress_reports")
	grantSQL = strings.ReplaceAll(grantSQL, `:"app_role"`, qr)
	_, e = f.db.Pool.Exec(f.ctx, grantSQL)
	must(t, e)
	// v11 must narrow the earlier bulk function grants exactly as documented.
	configureDocumentedBonusRole(t, f.ctx, f.db.Pool, role)
	pc := f.db.Pool.Config().Copy()
	pc.ConnConfig.User = role
	pc.ConnConfig.Password = secret
	pool, e = pgxpool.NewWithConfig(f.ctx, pc)
	must(t, e)
	var current string
	must(t, pool.QueryRow(f.ctx, `SELECT current_user`).Scan(&current))
	if current != role {
		t.Fatal("test did not use a separate unprivileged login")
	}
	must(t, database.ValidateApplicationRole(f.ctx, pool))
	progressRelation := pgx.Identifier{schema, "task_progress_reports"}.Sanitize()
	for _, privilege := range []struct {
		name string
		want bool
	}{
		{"SELECT", true},
		{"INSERT", true},
		{"UPDATE", false},
		{"DELETE", false},
		{"TRUNCATE", false},
	} {
		var allowed bool
		must(t, pool.QueryRow(f.ctx, `SELECT has_table_privilege(current_user,$1::text,$2::text)`, progressRelation, privilege.name).Scan(&allowed))
		if allowed != privilege.want {
			t.Fatalf("progress reports %s privilege = %t, want %t", privilege.name, allowed, privilege.want)
		}
	}
	var ignoreUpdateAllowed bool
	must(t, pool.QueryRow(f.ctx, `SELECT has_column_privilege(current_user,$1::text,'ignore_reason','UPDATE')`, pgx.Identifier{schema, "task_dependencies"}.Sanitize()).Scan(&ignoreUpdateAllowed))
	if !ignoreUpdateAllowed {
		t.Fatal("restricted app role cannot update ignored dependency fields")
	}
	// Preserve least privilege: changing a dependency's identity is still denied.
	if _, denied := pool.Exec(f.ctx, `UPDATE `+qs+`.task_dependencies SET predecessor_id=predecessor_id WHERE FALSE`); denied == nil {
		t.Fatal("restricted role unexpectedly gained full dependency UPDATE privileges")
	} else {
		var pe *pgconn.PgError
		if !errors.As(denied, &pe) || pe.Code != "42501" {
			t.Fatalf("unexpected dependency identity UPDATE failure: %v", denied)
		}
	}
	// Effective privileges, not role flags alone, must be checked.
	for _, grant := range []struct{ grant, revoke string }{
		{`GRANT UPDATE ON ` + qs + `.audit_events TO ` + qr, `REVOKE UPDATE ON ` + qs + `.audit_events FROM ` + qr},
		{`GRANT SET ON PARAMETER session_replication_role TO ` + qr, `REVOKE SET ON PARAMETER session_replication_role FROM ` + qr},
		{`GRANT CREATE ON SCHEMA ` + qs + ` TO ` + qr, `REVOKE CREATE ON SCHEMA ` + qs + ` FROM ` + qr},
		{`GRANT TRUNCATE ON ` + qs + `.contribution_cases TO ` + qr, `REVOKE TRUNCATE ON ` + qs + `.contribution_cases FROM ` + qr},
	} {
		_, e = f.db.Pool.Exec(f.ctx, grant.grant)
		must(t, e)
		if e = database.ValidateApplicationRole(f.ctx, pool); !errors.Is(e, database.ErrUnsafeApplicationRole) {
			t.Fatal("unsafe effective privilege passed production startup")
		}
		_, e = f.db.Pool.Exec(f.ctx, grant.revoke)
		must(t, e)
	}
	must(t, database.ValidateApplicationRole(f.ctx, pool))

	// Bonus reads use the same restricted login: SELECT is present, while
	// direct writes remain denied even with the v11 command workflow configured.
	for _, relation := range []string{"bonus_rounds", "bonus_round_versions", "bonus_allocation_lines", "bonus_member_totals"} {
		var allowed bool
		must(t, pool.QueryRow(f.ctx, `SELECT has_table_privilege(current_user,$1::text,'SELECT')`, pgx.Identifier{schema, relation}.Sanitize()).Scan(&allowed))
		if !allowed {
			t.Fatal("restricted app role cannot read a Phase06 bonus relation")
		}
	}
	if _, denied := pool.Exec(f.ctx, `INSERT INTO `+qs+`.bonus_rounds(team_id,project_id,funding_key,funding_label,funding_source,created_by)
		VALUES(gen_random_uuid(),gen_random_uuid(),'forbidden-round','Forbidden round','test',gen_random_uuid())`); denied == nil {
		t.Fatal("restricted app role unexpectedly gained bonus write privileges")
	} else {
		var pg *pgconn.PgError
		if !errors.As(denied, &pg) || pg.Code != "42501" {
			t.Fatalf("unexpected bonus write failure: %v", denied)
		}
	}

	for _, q := range []string{`SET session_replication_role=replica`, `CREATE TABLE forbidden_m3_ddl(id integer)`, `CREATE FUNCTION forbidden_m3_function() RETURNS integer LANGUAGE sql AS 'SELECT 1'`, `ALTER TABLE contribution_confirmations DISABLE TRIGGER ALL`, `TRUNCATE audit_events`, `UPDATE audit_events SET action='changed'`, `DELETE FROM contribution_confirmations`} {
		_, e = pool.Exec(f.ctx, q)
		var pg *pgconn.PgError
		if !errors.As(e, &pg) || pg.Code != "42501" {
			t.Fatal("ordinary application role retained unnecessary mutation or DDL privileges")
		}
	}
	// Seed a genuine Phase 03 dependency using the isolated fixture's owner
	// connection, then exercise its ignore endpoint via the restricted app role.
	predecessor, prepErr := f.tasks.Create(f.ctx, f.actors["lead"], f.team, f.project.ID, tasks.CreateInput{
		TaskFields: tasks.TaskFields{Title: "Restricted-role predecessor", OwnerID: f.actors["worker"].UserID}, Weight: 1,
	})
	must(t, prepErr)
	dependent, prepErr := f.tasks.Create(f.ctx, f.actors["lead"], f.team, f.project.ID, tasks.CreateInput{
		TaskFields: tasks.TaskFields{Title: "Restricted-role dependent", OwnerID: f.actors["worker"].UserID}, Weight: 1,
	})
	must(t, prepErr)
	dependency, prepErr := f.tasks.AddDependency(f.ctx, f.actors["lead"], f.team, f.project.ID, dependent.ID, predecessor.ID, dependent.Version)
	must(t, prepErr)
	dependent, prepErr = f.tasks.Get(f.ctx, f.actors["lead"], f.team, f.project.ID, dependent.ID)
	must(t, prepErr)

	logger := slog.New(slog.NewTextHandler(&f.logs, nil))
	ar := audit.NewRepository(pool)
	as := auth.NewService(auth.NewRepository(pool), ar, auth.NewLoginLimiter(100, time.Minute), time.Hour, logger)
	ps := projects.NewService(projects.NewRepository(pool), teams.NewRepository(pool), ar, logger)
	svc := contributions.NewService(contributions.NewRepository(pool), ps)
	taskSvc := tasks.NewService(tasks.NewRepository(), ps)
	f.handler = httpapi.New(config.Config{Environment: config.EnvironmentTest}, logger, system.NewRepository(pool), as, auth.NewHandler(as, false, time.Hour, logger), nil, tasks.NewHandler(taskSvc, logger), contributions.NewHandler(svc, logger)).Handler()
	// Use the same real HTTP login flow on the restricted connection, not SET ROLE
	// inside an owner session and not a fabricated middleware identity.
	f.login("lead")
	f.login("appraiser")
	f.login("worker")
	ignorePath := f.base() + "/tasks/" + dependent.ID.String() + "/dependencies/" + dependency.ID.String() + "/ignore"
	ignored := data[tasks.Dependency](t, f.do("POST", ignorePath, map[string]any{
		"reason":           "Ignore dependency with production-style restricted app credentials",
		"expected_version": dependent.Version,
	}, f.cookies["lead"], true), 200)
	if ignored.ID != dependency.ID || ignored.IgnoredAt == nil || ignored.IgnoredBy == nil || *ignored.IgnoredBy != f.actors["lead"].UserID {
		t.Fatal("restricted-role dependency-ignore HTTP endpoint did not persist its result")
	}
	f.grant("appraiser")
	c := f.create()
	f.prepare(c)
	conf := f.confirm(c)
	n := f.notice(c, conf)
	f.http("POST", f.path(c, "/notices/"+n.String()+"/receipt"), "worker", f.receiptInput(c, conf), 200)
	f.noCredentials()
}

func TestContributionApplicationRoleLegacySchemasAndV11Upgrade(t *testing.T) {
	f := testdb.New(t, os.Getenv)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	must(t, database.Status(ctx, f.SQL, slog.New(slog.NewTextHandler(io.Discard, nil))))
	role := "nexora_upgrade_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	qr := pgx.Identifier{role}.Sanitize()
	var schema string
	must(t, f.Pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema))
	qs := pgx.Identifier{schema}.Sanitize()
	secret := uuid.NewString() + uuid.NewString()
	_, e := f.Pool.Exec(ctx, `CREATE ROLE `+qr+` LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD '`+secret+`'`)
	must(t, e)
	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, e := f.Pool.Exec(cleanup, `REVOKE ALL ON ALL TABLES IN SCHEMA `+qs+` FROM `+qr+`; REVOKE ALL ON ALL FUNCTIONS IN SCHEMA `+qs+` FROM `+qr+`; REVOKE ALL ON SCHEMA `+qs+` FROM `+qr+`; DROP ROLE `+qr)
		if e != nil {
			t.Error("disposable upgrade role cleanup failed")
		}
	})
	cfg := f.Pool.Config().Copy()
	cfg.ConnConfig.User, cfg.ConnConfig.Password = role, secret
	pool, e = pgxpool.NewWithConfig(ctx, cfg)
	must(t, e)
	for _, migration := range []struct {
		name    string
		version int64
	}{{"v9", 9}, {"v10", 10}, {"v11", 11}} {
		t.Run(migration.name, func(t *testing.T) {
			must(t, goose.UpToContext(ctx, f.SQL, "migrations", migration.version))
			_, e := f.Pool.Exec(ctx, `GRANT USAGE ON SCHEMA `+qs+` TO `+qr+`; GRANT SELECT ON ALL TABLES IN SCHEMA `+qs+` TO `+qr+`; GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA `+qs+` TO `+qr)
			must(t, e)
			if migration.version == 11 {
				if e = database.ValidateApplicationRole(ctx, pool); !errors.Is(e, database.ErrUnsafeApplicationRole) {
					t.Fatalf("upgraded role must require v11 configuration: %v", e)
				}
				configureDocumentedBonusRole(t, ctx, f.Pool, role)
			}
			must(t, database.ValidateApplicationRole(ctx, pool))
		})
	}
}

func configureDocumentedBonusRole(t *testing.T, ctx context.Context, owner *pgxpool.Pool, role string) {
	t.Helper()
	doc, err := os.ReadFile("../../../../docs/operations/application-database-role.md")
	must(t, err)
	const statement = `SELECT bonus_configure_application_role(:'app_role'::name);`
	if !strings.Contains(string(doc), statement) {
		t.Fatal("production application-role grants omit the v11 configure step")
	}
	// Only adapt psql's literal parameter syntax; execute as the migration owner
	// in the same schema, after the generic grants, just like the deployment guide.
	_, err = owner.Exec(ctx, strings.ReplaceAll(statement, `:'app_role'`, "$1"), role)
	must(t, err)
}
