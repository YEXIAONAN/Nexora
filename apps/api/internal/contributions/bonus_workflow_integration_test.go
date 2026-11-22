//go:build integration

package contributions_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/audit"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/bonusresults"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/bonusrounds"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/contributions"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/teams"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type bonusFlow struct {
	*fixture
	service      *bonusrounds.Service
	round        uuid.UUID
	confirmation uuid.UUID
	caseID       uuid.UUID
}

func setupBonusFlow(t *testing.T) *bonusFlow {
	f := setup(t)
	f.grant("appraiser")
	c := f.create()
	f.prepare(c)
	confirmation := f.confirm(c)
	b := &bonusFlow{fixture: f, service: bonusrounds.NewService(bonusrounds.NewRepository(f.db.Pool)), confirmation: confirmation, caseID: c}
	b.round = b.createRound("school-2026", "member", "arbiter").RoundID
	return b
}
func (b *bonusFlow) createRound(key, manager, reviewer string) bonusrounds.CommandResult {
	b.t.Helper()
	out, e := b.service.Execute(b.ctx, b.actors["lead"], b.team, b.project.ID, uuid.Nil, "create", bonusrounds.CreateInput{FundingKey: key, FundingLabel: "Real funding award", FundingSource: "School", ManagerID: b.actors[manager].UserID, ReviewerID: b.actors[reviewer].UserID, Reason: "Explicit independent appointments", ClientRequestID: uuid.New()})
	must(b.t, e)
	return out
}
func (b *bonusFlow) command() bonusrounds.Command {
	b.t.Helper()
	var revision int
	must(b.t, b.db.Pool.QueryRow(b.ctx, `SELECT revision FROM bonus_rounds WHERE id=$1`, b.round).Scan(&revision))
	return bonusrounds.Command{ExpectedRevision: revision, ClientRequestID: uuid.New(), Reason: "Reviewed manual bonus command"}
}
func (b *bonusFlow) plan() bonusrounds.PlanInput {
	yes := true
	return bonusrounds.PlanInput{Command: b.command(), TotalBudgetMinor: 1000, Sources: []bonusrounds.PlanSourceInput{{ConfirmationID: b.confirmation, SourceBudgetMinor: 900, Reason: "Source award approved manually", Lines: []bonusrounds.PlanLineInput{{UserID: b.actors["worker"].UserID, Eligible: &yes, AmountMinor: 800, Reason: "Eligible confirmed contributor"}}}}}
}
func (b *bonusFlow) run(actor, action string, input any) bonusrounds.CommandResult {
	b.t.Helper()
	out, e := b.service.Execute(b.ctx, b.actors[actor], b.team, b.project.ID, b.round, action, input)
	must(b.t, e)
	return out
}
func (b *bonusFlow) workspace(actor string) bonusrounds.Workspace {
	b.t.Helper()
	out, e := b.service.Workspace(b.ctx, b.actors[actor], b.team, b.project.ID, b.round)
	must(b.t, e)
	return out
}
func (b *bonusFlow) mine() bonusresults.Page {
	b.t.Helper()
	out, e := bonusresults.NewService(bonusresults.NewRepository(b.db.Pool)).Mine(b.ctx, b.actors["worker"], b.team)
	must(b.t, e)
	return out
}
func (b *bonusFlow) approveAndPublish() {
	b.run("member", "submit", b.command())
	b.run("arbiter", "confirm", b.command())
	b.run("member", "publish", b.command())
}

func TestBonusWorkflowFullManualLifecycleAfterArchive(t *testing.T) {
	b := setupBonusFlow(t)
	notice := b.notice(b.caseID, b.confirmation)
	b.http("POST", b.path(b.caseID, "/notices/"+notice.String()+"/receipt"), "worker", b.receiptInput(b.caseID, b.confirmation), 200)
	project, e := b.ps.Get(b.ctx, b.actors["lead"], b.team, b.project.ID)
	must(t, e)
	_, e = b.ps.Archive(b.ctx, b.actors["lead"], b.team, b.project.ID, project.Version)
	must(t, e)
	// Bonus work does not mutate the archived project or contribution evidence.
	workspace, e := b.service.ProjectWorkspace(b.ctx, b.actors["lead"], b.team, b.project.ID)
	must(t, e)
	if !workspace.CanCreate || len(workspace.Sources) != 1 || workspace.Sources[0].ConfirmationID != b.confirmation {
		t.Fatalf("missing formal source: %+v", workspace)
	}
	_, e = b.service.Workspace(b.ctx, b.actors["lead"], b.team, b.project.ID, b.round)
	status(t, e, 404)
	_, e = b.service.ProjectWorkspace(b.ctx, b.actors["worker"], b.team, b.project.ID)
	status(t, e, 404)
	_, e = b.service.ProjectWorkspace(b.ctx, b.actors["root"], b.team, b.project.ID)
	status(t, e, 404)
	detail, e := b.service.Get(b.ctx, b.actors["worker"], b.team, b.project.ID, b.round)
	must(t, e)
	if detail.AmountsVisible {
		t.Fatal("metadata exposed money")
	}
	initial := b.workspace("member")
	if initial.Version.TotalBudgetMinor != 0 || len(initial.Version.Sources) != 0 {
		t.Fatal("creation was not an empty draft")
	}
	plan := b.plan()
	saved := b.run("member", "plan", plan)
	again := b.run("member", "plan", plan)
	if again != saved {
		t.Fatal("command replay changed result")
	}
	changed := plan
	changed.TotalBudgetMinor = 1001
	_, e = b.service.Execute(b.ctx, b.actors["member"], b.team, b.project.ID, b.round, "plan", changed)
	status(t, e, 409)
	current := b.workspace("member")
	if current.Version.PlanUnallocatedMinor != 100 || current.Version.Sources[0].SourceUnallocatedMinor != 100 || current.Version.Sources[0].Lines[0].AmountMinor != 800 {
		t.Fatal("two-level conservation lost")
	}
	b.run("member", "submit", b.command())
	_, e = b.service.Execute(b.ctx, b.actors["member"], b.team, b.project.ID, b.round, "plan", b.plan())
	status(t, e, 409)
	b.run("arbiter", "return", b.command())
	b.run("member", "submit", b.command())
	b.run("arbiter", "confirm", b.command())
	if len(b.mine().Items) != 0 {
		t.Fatal("unpublished amount exposed")
	}
	b.run("member", "publish", b.command())
	results := b.mine()
	if len(results.Items) != 1 || results.Items[0].AmountMinor != 800 || results.Items[0].PublishedAt.IsZero() || results.Items[0].PaymentStatus != "not_recorded" {
		t.Fatalf("published results: %+v", results)
	}
	payment := bonusrounds.PaymentInput{Command: b.command(), UserID: b.actors["worker"].UserID, Reference: "SIMULATED-001"}
	recorded := b.run("member", "payments", payment)
	if b.run("member", "payments", payment) != recorded {
		t.Fatal("payment replay changed revision")
	}
	payment.Command = b.command()
	_, e = b.service.Execute(b.ctx, b.actors["member"], b.team, b.project.ID, b.round, "payments", payment)
	status(t, e, 409)
	if b.mine().Items[0].PaymentStatus != "recorded" {
		t.Fatal("payment status did not come from record")
	}
	_, e = b.service.Execute(b.ctx, b.actors["member"], b.team, b.project.ID, b.round, "revise", b.command())
	status(t, e, 409)
	var raw, suggested, manual int64
	must(t, b.db.Pool.QueryRow(b.ctx, `SELECT sum(raw_rounded_minor)::bigint,sum(suggested_minor)::bigint,sum(final_minor)::bigint FROM bonus_allocation_lines WHERE version_id=(SELECT latest_version_id FROM bonus_rounds WHERE id=$1)`, b.round).Scan(&raw, &suggested, &manual))
	if raw != 0 || suggested != 0 || manual != 800 {
		t.Fatal("manual plan pretended to run an algorithm")
	}
	for _, query := range []string{`UPDATE bonus_payments SET reference='tampered' WHERE round_id=$1`, `DELETE FROM bonus_confirmations WHERE round_id=$1`, `UPDATE bonus_allocation_lines SET final_minor=1 WHERE round_id=$1`, `DELETE FROM bonus_adjustments WHERE round_id=$1`, `DELETE FROM bonus_events WHERE round_id=$1`, `DELETE FROM bonus_commands WHERE round_id=$1`} {
		if _, e = b.db.Pool.Exec(b.ctx, query, b.round); e == nil {
			t.Fatalf("immutable evidence accepted: %s", query)
		}
	}
	var state string
	must(t, b.db.Pool.QueryRow(b.ctx, `SELECT status FROM projects WHERE id=$1`, b.project.ID).Scan(&state))
	if state != "archived" {
		t.Fatal("bonus reopened archived project")
	}
}

func TestBonusWorkflowRevisionKeepsPublishedResultAndSnapshots(t *testing.T) {
	b := setupBonusFlow(t)
	b.run("member", "plan", b.plan())
	b.approveAndPublish()
	old := b.workspace("member").Version.ID
	b.run("member", "revise", b.command())
	if len(b.mine().Items) != 1 || b.mine().Items[0].AmountMinor != 800 {
		t.Fatal("draft revision hid effective result")
	}
	_, e := b.service.Execute(b.ctx, b.actors["member"], b.team, b.project.ID, b.round, "payments", bonusrounds.PaymentInput{Command: b.command(), UserID: b.actors["worker"].UserID, Reference: "must not pay during revision"})
	status(t, e, 409)
	plan := b.plan()
	plan.Sources[0].Lines[0].AmountMinor = 700
	b.run("member", "plan", plan)
	snapshot := b.workspace("member").Version.ID
	plan = b.plan()
	plan.Sources[0].Lines[0].AmountMinor = 600
	b.run("member", "plan", plan)
	var previous int64
	must(t, b.db.Pool.QueryRow(b.ctx, `SELECT final_minor FROM bonus_allocation_lines WHERE version_id=$1`, snapshot).Scan(&previous))
	if previous != 700 {
		t.Fatal("draft audit snapshot overwritten")
	}
	b.approveAndPublish()
	if b.mine().Items[0].AmountMinor != 600 {
		t.Fatal("new formal result not effective")
	}
	var oldState string
	must(t, b.db.Pool.QueryRow(b.ctx, `SELECT status FROM bonus_round_versions WHERE id=$1`, old).Scan(&oldState))
	if oldState != "superseded" {
		t.Fatal("old confirmation was overwritten")
	}
	// A different real funding batch can reuse the same contribution.
	b.round = b.createRound("college-2026", "member", "arbiter").RoundID
	b.run("member", "plan", b.plan())
	b.approveAndPublish()
	if len(b.mine().Items) != 2 {
		t.Fatal("independent funding rounds were merged")
	}
	_, e = b.service.Execute(b.ctx, b.actors["lead"], b.team, b.project.ID, uuid.Nil, "create", bonusrounds.CreateInput{FundingKey: "college-2026", FundingLabel: "Duplicate", FundingSource: "school", ManagerID: b.actors["member"].UserID, ReviewerID: b.actors["arbiter"].UserID, Reason: "Duplicate batch", ClientRequestID: uuid.New()})
	status(t, e, 409)
}

func TestBonusWorkflowIndependentReviewAndGrantEpoch(t *testing.T) {
	b := setupBonusFlow(t)
	// Contributor cannot confirm even if explicitly excluded and receiving zero.
	b.round = b.createRound("conflicted-worker", "member", "worker").RoundID
	plan := b.plan()
	no := false
	plan.Sources[0].Lines[0].Eligible = &no
	plan.Sources[0].Lines[0].AmountMinor = 0
	b.run("member", "plan", plan)
	b.run("member", "submit", b.command())
	if b.workspace("worker").Permissions.Confirm {
		t.Fatal("zero allocation bypassed beneficiary independence")
	}
	_, e := b.service.Execute(b.ctx, b.actors["worker"], b.team, b.project.ID, b.round, "confirm", b.command())
	status(t, e, 403)
	// Contribution confirmer also cannot review its own source.
	b.round = b.createRound("conflicted-appraiser", "member", "appraiser").RoundID
	b.run("member", "plan", b.plan())
	b.run("member", "submit", b.command())
	_, e = b.service.Execute(b.ctx, b.actors["appraiser"], b.team, b.project.ID, b.round, "confirm", b.command())
	status(t, e, 403)
	// Active epoch is checked on every read/write, including idempotent replays.
	b.round = b.createRound("epochs", "member", "arbiter").RoundID
	plan = b.plan()
	b.run("member", "plan", plan)
	_, e = b.ps.ChangeMember(b.ctx, b.actors["lead"], b.team, b.project.ID, b.actors["member"].UserID, false)
	must(t, e)
	_, e = b.ps.ChangeMember(b.ctx, b.actors["lead"], b.team, b.project.ID, b.actors["member"].UserID, true)
	must(t, e)
	_, e = b.service.Workspace(b.ctx, b.actors["member"], b.team, b.project.ID, b.round)
	status(t, e, 404)
	_, e = b.service.Execute(b.ctx, b.actors["member"], b.team, b.project.ID, b.round, "plan", plan)
	status(t, e, 403)
	for _, role := range []string{"editor", "budget", "eligibility"} {
		b.run("lead", "grants", bonusrounds.GrantInput{Command: b.command(), SubjectID: b.actors["member"].UserID, RoleCode: role})
	}
	if !b.workspace("member").Permissions.Edit {
		t.Fatal("explicit regrant did not restore authority")
	}
	_, e = b.service.Execute(b.ctx, b.actors["lead"], b.team, b.project.ID, b.round, "grants", bonusrounds.GrantInput{Command: b.command(), SubjectID: b.actors["member"].UserID, RoleCode: "confirmer"})
	status(t, e, 409)
	b.run("lead", "grants", bonusrounds.GrantInput{Command: b.command(), SubjectID: b.actors["member"].UserID, RoleCode: "budget", Revoke: true})
	_, e = b.service.Execute(b.ctx, b.actors["member"], b.team, b.project.ID, b.round, "submit", b.command())
	status(t, e, 403)
}

func TestBonusWorkflowEvidenceRevalidationAndDatabaseBoundaries(t *testing.T) {
	b := setupBonusFlow(t)
	b.run("member", "plan", b.plan())
	b.run("member", "submit", b.command())
	// A real dispute invalidates the source after submission; confirmation must fail.
	b.http("POST", b.path(b.caseID, "/disputes"), "worker", contributions.DisputeInput{VersionCommand: b.current(b.caseID), ConfirmationID: b.confirmation}, 201)
	_, e := b.service.Execute(b.ctx, b.actors["arbiter"], b.team, b.project.ID, b.round, "confirm", b.command())
	status(t, e, 409)
	if b.workspace("arbiter").Version.Status != "pending_review" {
		t.Fatal("failed review changed state")
	}
	// Call the security function directly to exercise DB validation, not Go checks.
	b.round = b.createRound("db-guards", "member", "arbiter").RoundID
	input := b.plan()
	input.Sources[0].Lines[0].UserID = b.actors["member"].UserID
	raw, e := json.Marshal(input)
	must(t, e)
	_, e = b.db.Pool.Exec(b.ctx, `SELECT bonus_workflow_command($1,$2,$3,$4,$5,'plan',$6::jsonb)`, b.team, b.project.ID, b.round, b.actors["member"].UserID, b.actors["member"].SessionID, raw)
	if e == nil {
		t.Fatal("database accepted forged source/share")
	}
	for _, value := range []string{"0.5", "-1", "900000000000001", "null", "\"100\""} {
		_, e = b.db.Pool.Exec(b.ctx, `SELECT bonus_minor($1::jsonb)`, value)
		if e == nil {
			t.Fatalf("database accepted money %s", value)
		}
	}
	_, e = b.service.Workspace(b.ctx, b.actors["arbiter"], uuid.New(), b.project.ID, b.round)
	status(t, e, 404)
	_, e = b.db.Pool.Exec(b.ctx, `UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=$1`, b.actors["arbiter"].SessionID)
	must(t, e)
	_, e = b.service.Workspace(b.ctx, b.actors["arbiter"], b.team, b.project.ID, b.round)
	status(t, e, 404)
}

func TestBonusWorkflowTeamDepartureCannotResurrectGrants(t *testing.T) {
	b := setupBonusFlow(t)
	_, e := b.db.Pool.Exec(b.ctx, `UPDATE team_members SET status='removed',removed_at=clock_timestamp() WHERE team_id=$1 AND user_id=$2`, b.team, b.actors["member"].UserID)
	must(t, e)
	_, e = b.db.Pool.Exec(b.ctx, `UPDATE team_members SET status='active',removed_at=NULL WHERE team_id=$1 AND user_id=$2`, b.team, b.actors["member"].UserID)
	must(t, e)
	_, e = b.service.Workspace(b.ctx, b.actors["member"], b.team, b.project.ID, b.round)
	status(t, e, 404)
	var active int
	must(t, b.db.Pool.QueryRow(b.ctx, `SELECT count(*) FROM bonus_role_grants WHERE round_id=$1 AND subject_id=$2 AND status='active'`, b.round, b.actors["member"].UserID).Scan(&active))
	if active != 0 {
		t.Fatal("team rejoin resurrected bonus grants")
	}
}

func TestBonusWorkflowExplicitContributionConflictAndDirectConfirmationGuard(t *testing.T) {
	b := setupBonusFlow(t)
	b.grant("arbiter")
	b.run("member", "plan", b.plan())
	b.run("member", "submit", b.command())
	b.http("POST", b.path(b.caseID, "/conflicts"), "arbiter", contributions.ConflictInput{Command: b.current(b.caseID).Command, UserID: b.actors["arbiter"].UserID, Kind: "financial_interest", Details: "Explicit review conflict"}, 201)
	_, e := b.service.Execute(b.ctx, b.actors["arbiter"], b.team, b.project.ID, b.round, "confirm", b.command())
	status(t, e, 403)
	_, e = b.db.Pool.Exec(b.ctx, `INSERT INTO bonus_confirmations(team_id,project_id,round_id,version_id,grant_id,confirmed_by,version_digest,reason,request_id)
 SELECT r.team_id,r.project_id,r.id,v.id,g.id,g.subject_id,v.submitted_digest,'must reject direct conflicted review','direct-review'
 FROM bonus_rounds r JOIN bonus_round_versions v ON v.id=r.latest_version_id JOIN bonus_role_grants g ON g.round_id=r.id AND g.role_code='confirmer' WHERE r.id=$1`, b.round)
	if e == nil {
		t.Fatal("database permitted conflicted confirmation without API")
	}
	if b.workspace("arbiter").Version.Status != "pending_review" {
		t.Fatal("no independent reviewer must leave plan pending")
	}
}

func TestBonusWorkflowConcurrentExpectedRevision(t *testing.T) {
	b := setupBonusFlow(t)
	first := b.plan()
	second := b.plan()
	second.Sources[0].Lines[0].AmountMinor = 700
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, p := range []bonusrounds.PlanInput{first, second} {
		wg.Add(1)
		go func(p bonusrounds.PlanInput) {
			defer wg.Done()
			<-start
			_, e := b.service.Execute(b.ctx, b.actors["member"], b.team, b.project.ID, b.round, "plan", p)
			results <- e
		}(p)
	}
	close(start)
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else {
			status(t, e, 409)
			conflicts++
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("success=%d conflicts=%d", success, conflicts)
	}
}

func setupRestrictedBonusRole(t *testing.T) (*bonusFlow, *pgxpool.Pool, string, string) {
	t.Helper()
	b := setupBonusFlow(t)
	role := "nexora_bonus_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	qr := pgx.Identifier{role}.Sanitize()
	var schema string
	must(t, b.db.Pool.QueryRow(b.ctx, `SELECT current_schema()`).Scan(&schema))
	qs := pgx.Identifier{schema}.Sanitize()
	secret := uuid.NewString() + uuid.NewString()
	_, e := b.db.Pool.Exec(b.ctx, `CREATE ROLE `+qr+` LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD '`+secret+`'`)
	must(t, e)
	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, e := b.db.Pool.Exec(ctx, `REVOKE ALL ON ALL TABLES IN SCHEMA `+qs+` FROM `+qr+`; REVOKE ALL ON ALL FUNCTIONS IN SCHEMA `+qs+` FROM `+qr+`; REVOKE ALL ON SCHEMA `+qs+` FROM `+qr+`; DROP ROLE `+qr)
		if e != nil {
			t.Error("disposable bonus role cleanup failed")
		}
	})
	_, e = b.db.Pool.Exec(b.ctx, `GRANT USAGE ON SCHEMA `+qs+` TO `+qr+`; GRANT SELECT ON ALL TABLES IN SCHEMA `+qs+` TO `+qr)
	must(t, e)
	cfg := b.db.Pool.Config().Copy()
	cfg.ConnConfig.User = role
	cfg.ConnConfig.Password = secret
	pool, e = pgxpool.NewWithConfig(b.ctx, cfg)
	must(t, e)
	var current string
	must(t, pool.QueryRow(b.ctx, `SELECT current_user`).Scan(&current))
	if current != role {
		t.Fatal("bonus role test requires a separate restricted LOGIN")
	}
	if e = database.ValidateApplicationRole(b.ctx, pool); !errors.Is(e, database.ErrUnsafeApplicationRole) {
		t.Fatalf("v11 without command/workspace grants must fail role validation: %v", e)
	}
	_, e = b.db.Pool.Exec(b.ctx, `GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA `+qs+` TO `+qr)
	must(t, e)
	if e = database.ValidateApplicationRole(b.ctx, pool); !errors.Is(e, database.ErrUnsafeApplicationRole) {
		t.Fatalf("bulk EXECUTE must not substitute for v11 configuration: %v", e)
	}
	configureDocumentedBonusRole(t, b.ctx, b.db.Pool, role)
	must(t, database.ValidateApplicationRole(b.ctx, pool))
	b.service = bonusrounds.NewService(bonusrounds.NewRepository(pool))
	return b, pool, qr, qs
}

func TestBonusWorkflowRestrictedApplicationRole(t *testing.T) {
	b, pool, _, _ := setupRestrictedBonusRole(t)
	b.round = b.createRound("restricted-role", "member", "arbiter").RoundID
	b.run("member", "plan", b.plan())
	b.approveAndPublish()
	b.run("member", "payments", bonusrounds.PaymentInput{Command: b.command(), UserID: b.actors["worker"].UserID, Reference: "SIMULATED-RESTRICTED"})
	if len(b.workspace("arbiter").Payments) != 1 {
		t.Fatal("restricted safe functions did not persist full workflow")
	}
	for _, q := range []string{`UPDATE bonus_rounds SET revision=revision+1`, `INSERT INTO bonus_payments DEFAULT VALUES`, `SELECT bonus_grant_internal(NULL,NULL,NULL,NULL,'editor',NULL,'forged','forged')`, `SELECT bonus_save_snapshot(NULL,NULL,'{}'::jsonb)`} {
		_, e := pool.Exec(b.ctx, q)
		var pg *pgconn.PgError
		if !errors.As(e, &pg) || pg.Code != "42501" {
			t.Fatalf("restricted bypass was not permission-denied: %v", e)
		}
	}
}

func TestBonusWorkflowRestrictedMembershipEpoch(t *testing.T) {
	b, pool, qr, qs := setupRestrictedBonusRole(t)
	// Exercise the deployment guide's column grant, including a real service
	// remove/re-add (the latter uses INSERT ... ON CONFLICT DO UPDATE).
	doc, e := os.ReadFile("../../../../docs/operations/application-database-role.md")
	must(t, e)
	const statement = `GRANT UPDATE (status,updated_at) ON project_members TO :"app_role";`
	if !strings.Contains(string(doc), statement) {
		t.Fatal("deployment guide must restrict project member UPDATE columns")
	}
	_, e = b.db.Pool.Exec(b.ctx, strings.ReplaceAll(strings.ReplaceAll(statement, "project_members", qs+".project_members"), `:"app_role"`, qr))
	must(t, e)
	_, e = b.db.Pool.Exec(b.ctx, `GRANT INSERT ON `+qs+`.project_members,`+qs+`.audit_events TO `+qr+`; GRANT UPDATE ON `+qs+`.teams TO `+qr)
	must(t, e)
	b.ps = projects.NewService(projects.NewRepository(pool), teams.NewRepository(pool), audit.NewRepository(pool), slog.New(slog.NewTextHandler(&b.logs, nil)))
	member := b.actors["member"].UserID
	epoch := func() int {
		t.Helper()
		var value int
		must(t, pool.QueryRow(b.ctx, `SELECT authorization_version FROM project_members WHERE team_id=$1 AND project_id=$2 AND user_id=$3`, b.team, b.project.ID, member).Scan(&value))
		return value
	}
	initial := epoch()
	_, e = pool.Exec(b.ctx, `UPDATE project_members SET authorization_version=authorization_version+1 WHERE team_id=$1 AND project_id=$2 AND user_id=$3`, b.team, b.project.ID, member)
	var pg *pgconn.PgError
	if !errors.As(e, &pg) || pg.Code != "42501" {
		t.Fatalf("documented role could write membership epoch: %v", e)
	}
	_, e = pool.Exec(b.ctx, `UPDATE project_members SET status=status,updated_at=clock_timestamp() WHERE team_id=$1 AND project_id=$2 AND user_id=$3`, b.team, b.project.ID, member)
	must(t, e)
	if epoch() != initial {
		t.Fatal("unchanged membership status advanced the epoch")
	}
	_, e = b.ps.ChangeMember(b.ctx, b.actors["lead"], b.team, b.project.ID, member, false)
	must(t, e)
	if epoch() != initial+1 {
		t.Fatal("removal must increment the epoch exactly once")
	}
	_, e = b.ps.ChangeMember(b.ctx, b.actors["lead"], b.team, b.project.ID, member, true)
	must(t, e)
	if epoch() != initial+2 {
		t.Fatal("re-add must increment the epoch exactly once")
	}
	// Defense in depth: even the old whole-table UPDATE grant cannot rewind or
	// advance an epoch, with or without naming status in the same statement.
	_, e = b.db.Pool.Exec(b.ctx, `GRANT UPDATE ON `+qs+`.project_members TO `+qr)
	must(t, e)
	for _, assignment := range []string{
		`authorization_version=$4`,
		`authorization_version=authorization_version+1`,
		`authorization_version=NULL`,
		`status='removed',authorization_version=$4`,
		`status=status,authorization_version=$4`,
	} {
		query := `UPDATE project_members SET ` + assignment + ` WHERE team_id=$1 AND project_id=$2 AND user_id=$3`
		args := []any{b.team, b.project.ID, member}
		if strings.Contains(assignment, "$4") {
			args = append(args, initial)
		}
		_, e = pool.Exec(b.ctx, query, args...)
		if !errors.As(e, &pg) || pg.Code != "23514" {
			t.Fatalf("epoch tampering did not hit the v11 guard (%s): %v", assignment, e)
		}
	}
	if epoch() != initial+2 {
		t.Fatal("failed tampering changed the membership epoch")
	}
	var historicalGrants int
	must(t, pool.QueryRow(b.ctx, `SELECT count(*) FROM bonus_role_grants WHERE round_id=$1 AND subject_id=$2 AND status='active' AND member_authorization_version=$3`, b.round, member, initial).Scan(&historicalGrants))
	if historicalGrants != 3 {
		t.Fatal("historical grants must remain as evidence, not be overwritten")
	}
	_, e = b.service.Workspace(b.ctx, b.actors["member"], b.team, b.project.ID, b.round)
	status(t, e, 404)
	_, e = b.service.Execute(b.ctx, b.actors["member"], b.team, b.project.ID, b.round, "plan", b.plan())
	status(t, e, 403)
}

func TestBonusWorkflowRestrictedTriggerRebinding(t *testing.T) {
	b, pool, qr, qs := setupRestrictedBonusRole(t)
	fingerprint := func() string {
		t.Helper()
		var value string
		must(t, b.db.Pool.QueryRow(b.ctx, `SELECT jsonb_build_object('round',to_jsonb(r),
 'grants',(SELECT jsonb_agg(to_jsonb(g) ORDER BY g.id) FROM bonus_role_grants g WHERE g.round_id=r.id),
 'events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY e.id) FROM bonus_events e WHERE e.round_id=r.id))::text
 FROM bonus_rounds r WHERE r.id=$1`, b.round).Scan(&value))
		return value
	}
	before := fingerprint()
	_, e := b.db.Pool.Exec(b.ctx, `GRANT EXECUTE ON FUNCTION `+qs+`.bonus_membership_invalidate() TO `+qr)
	must(t, e)
	if e = database.ValidateApplicationRole(b.ctx, pool); !errors.Is(e, database.ErrUnsafeApplicationRole) {
		t.Fatalf("trigger EXECUTE misconfiguration passed role validation: %v", e)
	}
	// A separate unprivileged login can own temp tables. Use the identical name
	// to also test search_path shadowing; the trigger must compare real relation OIDs.
	tx, e := pool.Begin(b.ctx)
	must(t, e)
	defer tx.Rollback(b.ctx)
	_, e = tx.Exec(b.ctx, `CREATE TEMP TABLE team_members(team_id uuid,user_id uuid,status text) ON COMMIT DROP`)
	must(t, e)
	_, e = tx.Exec(b.ctx, `INSERT INTO pg_temp.team_members VALUES($1,$2,'active')`, b.team, b.actors["member"].UserID)
	must(t, e)
	_, e = tx.Exec(b.ctx, `CREATE TRIGGER forged_departure AFTER UPDATE OF status ON pg_temp.team_members FOR EACH ROW EXECUTE FUNCTION `+qs+`.bonus_membership_invalidate()`)
	must(t, e)
	_, e = tx.Exec(b.ctx, `UPDATE pg_temp.team_members SET status='removed'`)
	var pg *pgconn.PgError
	if !errors.As(e, &pg) || pg.Code != "42501" || !strings.Contains(pg.Message, "real team membership trigger") {
		t.Fatalf("forged trigger did not hit the relation binding guard: %v", e)
	}
	must(t, tx.Rollback(b.ctx))
	if fingerprint() != before {
		t.Fatal("temporary-table trigger changed real grants, revision or events")
	}
	_, e = b.db.Pool.Exec(b.ctx, `REVOKE EXECUTE ON FUNCTION `+qs+`.bonus_membership_invalidate() FROM `+qr+`; GRANT UPDATE ON `+qs+`.team_members,`+qs+`.teams TO `+qr+`; GRANT UPDATE (status,updated_at) ON `+qs+`.project_members TO `+qr)
	must(t, e)
	must(t, database.ValidateApplicationRole(b.ctx, pool))
	// Legitimate table triggers still work without granting function EXECUTE.
	_, e = pool.Exec(b.ctx, `UPDATE team_members SET status='removed',removed_at=clock_timestamp() WHERE team_id=$1 AND user_id=$2`, b.team, b.actors["member"].UserID)
	must(t, e)
	_, e = pool.Exec(b.ctx, `UPDATE team_members SET status='active',removed_at=NULL WHERE team_id=$1 AND user_id=$2`, b.team, b.actors["member"].UserID)
	must(t, e)
	var active int
	must(t, pool.QueryRow(b.ctx, `SELECT count(*) FROM bonus_role_grants WHERE round_id=$1 AND subject_id=$2 AND status='active'`, b.round, b.actors["member"].UserID).Scan(&active))
	if active != 0 {
		t.Fatal("real team departure/rejoin resurrected grants")
	}
	_, e = b.service.Workspace(b.ctx, b.actors["member"], b.team, b.project.ID, b.round)
	status(t, e, 404)
}

func TestBonusWorkflowApplicationRolePrivileges(t *testing.T) {
	b, pool, qr, qs := setupRestrictedBonusRole(t)
	// Check effective ACLs individually: legal SELECT plus both exact entry
	// points, no direct DML (even column grants), and no bonus helper EXECUTE.
	for _, change := range []struct{ name, apply, undo string }{
		{"missing command", `REVOKE EXECUTE ON FUNCTION ` + qs + `.bonus_workflow_command(uuid,uuid,uuid,uuid,uuid,text,jsonb) FROM ` + qr, `GRANT EXECUTE ON FUNCTION ` + qs + `.bonus_workflow_command(uuid,uuid,uuid,uuid,uuid,text,jsonb) TO ` + qr},
		{"missing workspace", `REVOKE EXECUTE ON FUNCTION ` + qs + `.bonus_workflow_workspace(uuid,uuid,uuid,uuid,uuid) FROM ` + qr, `GRANT EXECUTE ON FUNCTION ` + qs + `.bonus_workflow_workspace(uuid,uuid,uuid,uuid,uuid) TO ` + qr},
		{"missing payments SELECT", `REVOKE SELECT ON ` + qs + `.bonus_payments FROM ` + qr, `GRANT SELECT ON ` + qs + `.bonus_payments TO ` + qr},
		{"missing commands SELECT", `REVOKE SELECT ON ` + qs + `.bonus_commands FROM ` + qr, `GRANT SELECT ON ` + qs + `.bonus_commands TO ` + qr},
		{"table INSERT", `GRANT INSERT ON ` + qs + `.bonus_commands TO ` + qr, `REVOKE INSERT ON ` + qs + `.bonus_commands FROM ` + qr},
		{"table UPDATE", `GRANT UPDATE ON ` + qs + `.bonus_rounds TO ` + qr, `REVOKE UPDATE ON ` + qs + `.bonus_rounds FROM ` + qr},
		{"table DELETE", `GRANT DELETE ON ` + qs + `.bonus_events TO ` + qr, `REVOKE DELETE ON ` + qs + `.bonus_events FROM ` + qr},
		{"column UPDATE", `GRANT UPDATE (revision) ON ` + qs + `.bonus_rounds TO ` + qr, `REVOKE UPDATE (revision) ON ` + qs + `.bonus_rounds FROM ` + qr},
		{"helper EXECUTE", `GRANT EXECUTE ON FUNCTION ` + qs + `.bonus_grant_internal(uuid,uuid,uuid,uuid,text,uuid,text,text) TO ` + qr, `REVOKE EXECUTE ON FUNCTION ` + qs + `.bonus_grant_internal(uuid,uuid,uuid,uuid,text,uuid,text,text) FROM ` + qr},
		{"v10 trigger EXECUTE", `GRANT EXECUTE ON FUNCTION ` + qs + `.bonus_m2_immutable_guard() TO ` + qr, `REVOKE EXECUTE ON FUNCTION ` + qs + `.bonus_m2_immutable_guard() FROM ` + qr},
	} {
		t.Run(change.name, func(t *testing.T) {
			_, e := b.db.Pool.Exec(b.ctx, change.apply)
			must(t, e)
			e = database.ValidateApplicationRole(b.ctx, pool)
			if !errors.Is(e, database.ErrUnsafeApplicationRole) {
				t.Errorf("incorrect effective v11 privilege passed role validation: %v", e)
			}
			_, e = b.db.Pool.Exec(b.ctx, change.undo)
			must(t, e)
			must(t, database.ValidateApplicationRole(b.ctx, pool))
		})
	}
}
