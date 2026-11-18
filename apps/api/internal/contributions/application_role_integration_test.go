//go:build integration

package contributions_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/audit"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/auth"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/config"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/contributions"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpapi"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/system"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/teams"
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
	_, e = f.db.Pool.Exec(f.ctx, `GRANT USAGE ON SCHEMA `+qs+` TO `+qr+`; GRANT SELECT ON ALL TABLES IN SCHEMA `+qs+` TO `+qr+`; GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA `+qs+` TO `+qr+`; GRANT INSERT ON `+qs+`.sessions,`+qs+`.audit_events,`+qs+`.contribution_appraisers,`+qs+`.contribution_cases,`+qs+`.contribution_case_versions,`+qs+`.contribution_shares,`+qs+`.contribution_share_adjustments,`+qs+`.contribution_confirmations,`+qs+`.contribution_conflicts,`+qs+`.contribution_disputes,`+qs+`.contribution_dispute_resolutions,`+qs+`.contribution_case_events,`+qs+`.contribution_notifications,`+qs+`.contribution_attestations TO `+qr+`; GRANT UPDATE ON `+qs+`.teams,`+qs+`.sessions,`+qs+`.contribution_appraisers,`+qs+`.contribution_cases,`+qs+`.contribution_case_versions,`+qs+`.contribution_shares TO `+qr+`; GRANT DELETE ON `+qs+`.contribution_shares TO `+qr)
	must(t, e)
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

	for _, q := range []string{`SET session_replication_role=replica`, `CREATE TABLE forbidden_m3_ddl(id integer)`, `CREATE FUNCTION forbidden_m3_function() RETURNS integer LANGUAGE sql AS 'SELECT 1'`, `ALTER TABLE contribution_confirmations DISABLE TRIGGER ALL`, `TRUNCATE audit_events`, `UPDATE audit_events SET action='changed'`, `DELETE FROM contribution_confirmations`} {
		_, e = pool.Exec(f.ctx, q)
		var pg *pgconn.PgError
		if !errors.As(e, &pg) || pg.Code != "42501" {
			t.Fatal("ordinary application role retained unnecessary mutation or DDL privileges")
		}
	}
	logger := slog.New(slog.NewTextHandler(&f.logs, nil))
	ar := audit.NewRepository(pool)
	as := auth.NewService(auth.NewRepository(pool), ar, auth.NewLoginLimiter(100, time.Minute), time.Hour, logger)
	ps := projects.NewService(projects.NewRepository(pool), teams.NewRepository(pool), ar, logger)
	svc := contributions.NewService(contributions.NewRepository(pool), ps)
	f.handler = httpapi.New(config.Config{Environment: config.EnvironmentTest}, logger, system.NewRepository(pool), as, auth.NewHandler(as, false, time.Hour, logger), nil, contributions.NewHandler(svc, logger)).Handler()
	// Use the same real HTTP login flow on the restricted connection, not SET ROLE
	// inside an owner session and not a fabricated middleware identity.
	f.login("lead")
	f.login("appraiser")
	f.login("worker")
	f.grant("appraiser")
	c := f.create()
	f.prepare(c)
	conf := f.confirm(c)
	n := f.notice(c, conf)
	f.http("POST", f.path(c, "/notices/"+n.String()+"/receipt"), "worker", f.receiptInput(c, conf), 200)
	f.noCredentials()
}
