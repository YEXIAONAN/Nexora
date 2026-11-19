//go:build integration

package contributions_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/contributions"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

func TestServiceContributionIdempotenceAndVersions(t *testing.T) {
	f := setup(t)
	in := contributions.CreateInput{TaskID: f.task.ID, ReviewID: f.review.ID, ClientRequestID: uuid.New(), Reason: "One real source"}
	a, e := f.svc.Create(f.ctx, f.actors["lead"], f.team, f.project.ID, in)
	must(t, e)
	b, e := f.svc.Create(f.ctx, f.actors["lead"], f.team, f.project.ID, in)
	must(t, e)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("create replay changed result")
	}
	changed := in
	changed.Reason = "different payload"
	_, e = f.svc.Create(f.ctx, f.actors["lead"], f.team, f.project.ID, changed)
	status(t, e, 409)
	in.ClientRequestID = uuid.New()
	b, e = f.svc.Create(f.ctx, f.actors["lead"], f.team, f.project.ID, in)
	must(t, e)
	if a.ID != b.ID {
		t.Fatal("review idempotency failed")
	}
	c := a.ID
	share := contributions.ShareInput{VersionCommand: f.current(c), ShareBPS: 4000, Basis: "Manual initial share", EvidenceRefs: []contributions.EvidenceReference{}}
	ctx := reqctx.WithRequestID(f.ctx, "m3-real-request-correlation")
	a, e = f.svc.SetShare(ctx, f.actors["lead"], f.team, f.project.ID, c, f.actors["worker"].UserID, share, false)
	must(t, e)
	b, e = f.svc.SetShare(ctx, f.actors["lead"], f.team, f.project.ID, c, f.actors["worker"].UserID, share, false)
	must(t, e)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("share replay repeated a mutation")
	}
	share.ShareBPS = 8000
	_, e = f.svc.SetShare(ctx, f.actors["lead"], f.team, f.project.ID, c, f.actors["worker"].UserID, share, false)
	status(t, e, 409)
	share.ClientRequestID = uuid.New()
	_, e = f.svc.SetShare(ctx, f.actors["lead"], f.team, f.project.ID, c, f.actors["worker"].UserID, share, false)
	status(t, e, 409)
	share.VersionCommand = f.current(c)
	share.CaseVersionID = uuid.New()
	_, e = f.svc.SetShare(ctx, f.actors["lead"], f.team, f.project.ID, c, f.actors["worker"].UserID, share, false)
	status(t, e, 404)
	var ok bool
	must(t, f.db.Pool.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM audit_events WHERE actor_id=$1 AND request_id='m3-real-request-correlation' AND action='contribution.share_adjusted')`, f.actors["lead"].UserID).Scan(&ok))
	if !ok {
		t.Fatal("server actor and Request ID were not recorded")
	}
	if f.detail(c).Latest.TotalBPS != 4000 {
		t.Fatal("stale or different-content command changed allocation")
	}
}
func TestServiceContributionAuthorizationEpoch(t *testing.T) {
	f := setup(t)
	f.grant("appraiser")
	c := f.create()
	f.prepare(c)
	_, e := f.ps.ChangeMember(f.ctx, f.actors["lead"], f.team, f.project.ID, f.actors["appraiser"].UserID, false)
	must(t, e)
	_, e = f.svc.VersionAction(f.ctx, f.actors["appraiser"], f.team, f.project.ID, c, "confirm", f.current(c))
	status(t, e, 404)
	_, e = f.ps.ChangeMember(f.ctx, f.actors["lead"], f.team, f.project.ID, f.actors["appraiser"].UserID, true)
	must(t, e)
	_, e = f.svc.VersionAction(f.ctx, f.actors["appraiser"], f.team, f.project.ID, c, "confirm", f.current(c))
	status(t, e, 404)
	grant := contributions.Command{ExpectedVersion: 1, ClientRequestID: uuid.New(), Reason: "Explicit grant in new membership epoch"}
	_, e = f.svc.SetAppraiser(f.ctx, f.actors["lead"], f.team, f.project.ID, f.actors["appraiser"].UserID, grant, false)
	must(t, e)
	revoke := contributions.Command{ExpectedVersion: 2, ClientRequestID: uuid.New(), Reason: "Withdraw appraisal authority"}
	_, e = f.svc.SetAppraiser(f.ctx, f.actors["lead"], f.team, f.project.ID, f.actors["appraiser"].UserID, revoke, true)
	must(t, e)
	_, e = f.svc.VersionAction(f.ctx, f.actors["appraiser"], f.team, f.project.ID, c, "confirm", f.current(c))
	status(t, e, 404)
	grant.ExpectedVersion = 3
	grant.ClientRequestID = uuid.New()
	_, e = f.svc.SetAppraiser(f.ctx, f.actors["lead"], f.team, f.project.ID, f.actors["appraiser"].UserID, grant, false)
	must(t, e)
	f.confirm(c)
	must(t, f.ts.RemoveMember(f.ctx, f.actors["root"], f.team, f.actors["worker"].UserID))
	_, e = f.svc.Get(f.ctx, f.actors["worker"], f.team, f.project.ID, c)
	status(t, e, 404)
	d := f.detail(c)
	if d.Effective == nil || len(d.Effective.Shares) != 1 || d.Effective.Shares[0].UserID != f.actors["worker"].UserID || f.official(c) != 1 {
		t.Fatal("departure erased true historical contribution")
	}
}
func TestServiceContributionIndependentProposerAndConflicts(t *testing.T) {
	f := setup(t)
	f.grant("appraiser")
	f.grant("arbiter")
	f.grant("worker")
	c := f.create()
	f.prepare(c)
	_, e := f.svc.VersionAction(f.ctx, f.actors["worker"], f.team, f.project.ID, c, "confirm", f.current(c))
	status(t, e, 403)
	// Explicitly authorized lead is still the source reviewer and proposer.
	_, e = f.svc.SetAppraiser(f.ctx, f.actors["appraiser"], f.team, f.project.ID, f.actors["lead"].UserID, contributions.Command{ExpectedVersion: 0, ClientRequestID: uuid.New(), Reason: "Explicit permission is not COI exemption"}, false)
	must(t, e)
	_, e = f.svc.VersionAction(f.ctx, f.actors["lead"], f.team, f.project.ID, c, "confirm", f.current(c))
	status(t, e, 403)
	f.confirm(c)
	_, e = f.svc.VersionAction(f.ctx, f.actors["appraiser"], f.team, f.project.ID, c, "revisions", f.current(c))
	must(t, e)
	f.share(c, 10000)
	_, e = f.svc.VersionAction(f.ctx, f.actors["appraiser"], f.team, f.project.ID, c, "submit", f.current(c))
	must(t, e)
	_, e = f.svc.VersionAction(f.ctx, f.actors["appraiser"], f.team, f.project.ID, c, "confirm", f.current(c))
	status(t, e, 403)
	_, e = f.svc.RecordConflict(f.ctx, f.actors["lead"], f.team, f.project.ID, c, contributions.ConflictInput{Command: f.current(c).Command, UserID: f.actors["arbiter"].UserID, Kind: "direct_interest", Details: "Direct interest was disclosed"})
	must(t, e)
	_, e = f.svc.VersionAction(f.ctx, f.actors["arbiter"], f.team, f.project.ID, c, "confirm", f.current(c))
	status(t, e, 403)
	if f.detail(c).Latest.Status != "pending_review" {
		t.Fatal("no qualified appraiser was bypassed")
	}
	f.grant("member")
	_, e = f.svc.VersionAction(f.ctx, f.actors["member"], f.team, f.project.ID, c, "confirm", f.current(c))
	must(t, e)
}

var businessTables = []string{"contribution_appraisers", "contribution_cases", "contribution_case_versions", "contribution_shares", "contribution_share_adjustments", "contribution_confirmations", "contribution_conflicts", "contribution_disputes", "contribution_dispute_resolutions", "contribution_case_events", "contribution_notifications", "contribution_attestations", "audit_events"}

func (f *fixture) snapshot() map[string]string {
	f.t.Helper()
	out := map[string]string{}
	for _, table := range businessTables {
		var raw string
		must(f.t, f.db.Pool.QueryRow(f.ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]')::text FROM `+table+` t`).Scan(&raw))
		sum := sha256.Sum256([]byte(raw))
		out[table] = hex.EncodeToString(sum[:])
	}
	return out
}
func (f *fixture) failCommand(key uuid.UUID) {
	f.t.Helper()
	_, e := f.db.Pool.Exec(f.ctx, `CREATE FUNCTION m3_fail_command() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='contribution.command_completed' AND NEW.details->>'client_request_id'='`+key.String()+`' THEN RAISE EXCEPTION 'PRIVATE_DATABASE_DETAIL_MUST_NOT_ESCAPE'; END IF; RETURN NEW; END $$; CREATE TRIGGER m3_fail_command BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION m3_fail_command()`)
	must(f.t, e)
}
func (f *fixture) clearFailure() {
	f.t.Helper()
	_, e := f.db.Pool.Exec(f.ctx, `DROP TRIGGER m3_fail_command ON audit_events; DROP FUNCTION m3_fail_command()`)
	must(f.t, e)
}
func TestServiceContributionAuditFailureIsAtomicAndSanitized(t *testing.T) {
	f := setup(t)
	f.grant("appraiser")
	f.grant("arbiter")
	c := f.create()
	f.prepare(c)
	in := f.current(c)
	before := f.snapshot()
	f.failCommand(in.ClientRequestID)
	_, e := f.svc.VersionAction(f.ctx, f.actors["appraiser"], f.team, f.project.ID, c, "confirm", in)
	status(t, e, 500)
	if !reflect.DeepEqual(before, f.snapshot()) {
		t.Fatal("command receipt failure left a partial confirmation")
	}
	f.clearFailure()
	confirmation := f.http("POST", f.path(c, "/confirm"), "appraiser", in, 200).ID
	n := f.notice(c, confirmation)
	receipt := f.receiptInput(c, confirmation)
	before = f.snapshot()
	f.failCommand(receipt.ClientRequestID)
	rr := f.do("POST", f.path(c, "/notices/"+n.String()+"/receipt"), receipt, f.cookies["worker"], true)
	data[any](t, rr, 500)
	if strings.Contains(rr.Body.String(), "PRIVATE_DATABASE_DETAIL") || strings.Contains(f.logs.String(), "PRIVATE_DATABASE_DETAIL") {
		t.Fatal("database details escaped sanitized error boundary")
	}
	if !reflect.DeepEqual(before, f.snapshot()) {
		t.Fatal("failed receipt left delivery or audit data")
	}
	f.clearFailure()
	f.http("POST", f.path(c, "/notices/"+n.String()+"/receipt"), "worker", receipt, 200)
	// A failed Go command receipt must also roll back native overturn + successor.
	dispute, e := f.svc.RaiseDispute(f.ctx, f.actors["worker"], f.team, f.project.ID, c, contributions.DisputeInput{VersionCommand: f.current(c), ConfirmationID: confirmation})
	must(t, e)
	ruling := contributions.ResolutionInput{Command: f.current(c).Command, Decision: "overturn"}
	before = f.snapshot()
	f.failCommand(ruling.ClientRequestID)
	_, e = f.svc.Resolve(f.ctx, f.actors["arbiter"], f.team, f.project.ID, c, dispute.ID, ruling)
	status(t, e, 500)
	if !reflect.DeepEqual(before, f.snapshot()) {
		t.Fatal("failed command left a partial overturn or successor draft")
	}
	f.clearFailure()
	_, e = f.svc.Resolve(f.ctx, f.actors["arbiter"], f.team, f.project.ID, c, dispute.ID, ruling)
	must(t, e)
	f.noCredentials()
}
func TestServiceContributionNoticeIntentDeduplication(t *testing.T) {
	f := setup(t)
	f.grant("appraiser")
	c := f.create()
	f.prepare(c)
	confirmation := f.confirm(c)
	input := func() contributions.NoticeInput {
		return contributions.NoticeInput{VersionCommand: f.current(c), ConfirmationID: confirmation, RecipientID: f.actors["worker"].UserID}
	}
	firstInput := input()
	first := f.http("POST", f.path(c, "/notices"), "lead", firstInput, 201)
	replay := f.http("POST", f.path(c, "/notices"), "lead", firstInput, 201)
	if replay.ID != first.ID {
		t.Fatal("same client_request_id created another notice")
	}
	// A lost response is retried by the browser with a new idempotency key but
	// the same confirmed result and recipient: it is still one business notice.
	retry := f.http("POST", f.path(c, "/notices"), "lead", input(), 201)
	if retry.ID != first.ID {
		t.Fatal("the same confirmed result and recipient created a duplicate notice")
	}
	count := func(confirmation uuid.UUID) int {
		var n int
		must(t, f.db.Pool.QueryRow(f.ctx, `SELECT count(*) FROM contribution_notifications WHERE team_id=$1 AND project_id=$2 AND task_id=$3 AND case_id=$4 AND confirmation_id=$5 AND recipient_id=$6 AND channel='authenticated_ack'`, f.team, f.project.ID, f.task.ID, c, confirmation, f.actors["worker"].UserID).Scan(&n))
		return n
	}
	if n := count(confirmation); n != 1 {
		t.Fatalf("duplicate notice rows=%d, want 1", n)
	}
	// Existing rows from before M5 must stay immutable. A new request resolves
	// to the earliest record and does not add a third duplicate.
	_, e := f.db.Pool.Exec(f.ctx, `INSERT INTO contribution_notifications(team_id,project_id,task_id,case_id,case_version_id,confirmation_id,recipient_id,channel,created_by,request_id) VALUES($1,$2,$3,$4,$5,$6,$7,'authenticated_ack',$8,'legacy-duplicate-intent')`, f.team, f.project.ID, f.task.ID, c, firstInput.CaseVersionID, confirmation, f.actors["worker"].UserID, f.actors["lead"].UserID)
	must(t, e)
	legacy := f.http("POST", f.path(c, "/notices"), "lead", input(), 201)
	if legacy.ID != first.ID {
		t.Fatal("dedupe did not select the earliest existing notice")
	}
	if n := count(confirmation); n != 2 {
		t.Fatalf("legacy duplicates were deleted or a third row was added: rows=%d", n)
	}
	in := f.receiptInput(c, confirmation)
	f.http("POST", f.path(c, "/notices/"+first.ID.String()+"/receipt"), "worker", in, 200)
	var bound bool
	must(t, f.db.Pool.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM contribution_attestations WHERE notice_id=$1 AND confirmation_id=$2 AND case_version_id=$3 AND subject_id=$4 AND purpose='delivery')`, first.ID, confirmation, in.CaseVersionID, f.actors["worker"].UserID).Scan(&bound))
	if !bound {
		t.Fatal("receipt did not bind the displayed confirmation and version")
	}
	// A new confirmed revision is a separate business intent and must be told
	// again; the earlier receipt cannot satisfy the new effective version.
	f.http("POST", f.path(c, "/revisions"), "lead", f.current(c), 200)
	f.prepare(c)
	confirmation = f.confirm(c)
	next := f.notice(c, confirmation)
	if next == first.ID {
		t.Fatal("a new confirmed revision reused the previous notice")
	}
	if n := count(confirmation); n != 1 {
		t.Fatalf("new revision notices=%d, want 1", n)
	}
	_, e = f.ps.Archive(f.ctx, f.actors["lead"], f.team, f.project.ID, f.project.Version)
	status(t, e, 409)
	f.http("POST", f.path(c, "/notices/"+next.String()+"/receipt"), "worker", f.receiptInput(c, confirmation), 200)
	_, e = f.ps.Archive(f.ctx, f.actors["lead"], f.team, f.project.ID, f.project.Version)
	must(t, e)
}

func TestServiceContributionNoticeHistoricalIntentIsNotNewDelivery(t *testing.T) {
	f := setup(t)
	f.grant("appraiser")
	c := f.create()
	f.prepare(c)
	firstConfirmation := f.confirm(c)
	firstVersion := *f.detail(c).Case.EffectiveConfirmedVersionID
	firstNotice := f.notice(c, firstConfirmation)
	f.http("POST", f.path(c, "/notices/"+firstNotice.String()+"/receipt"), "worker", f.receiptInput(c, firstConfirmation), 200)
	f.http("POST", f.path(c, "/revisions"), "lead", f.current(c), 200)
	f.prepare(c)
	secondConfirmation := f.confirm(c)
	// A stale request for the superseded confirmation resolves to its immutable
	// historical notice; it does not create a new notice or new delivery proof.
	oldCommand := f.current(c)
	oldCommand.CaseVersionID = firstVersion
	old := f.http("POST", f.path(c, "/notices"), "lead", contributions.NoticeInput{VersionCommand: oldCommand, ConfirmationID: firstConfirmation, RecipientID: f.actors["worker"].UserID}, 201)
	if old.ID != firstNotice {
		t.Fatal("historical notice intent was not resolved to its immutable notice")
	}
	var historical int
	must(t, f.db.Pool.QueryRow(f.ctx, `SELECT count(*) FROM contribution_notifications WHERE case_id=$1 AND confirmation_id=$2`, c, firstConfirmation).Scan(&historical))
	if historical != 1 {
		t.Fatalf("historical notices=%d, want 1", historical)
	}
	_, e := f.ps.Archive(f.ctx, f.actors["lead"], f.team, f.project.ID, f.project.Version)
	status(t, e, 409)
	next := f.notice(c, secondConfirmation)
	if next == firstNotice {
		t.Fatal("new confirmation reused the historical notice")
	}
	f.http("POST", f.path(c, "/notices/"+next.String()+"/receipt"), "worker", f.receiptInput(c, secondConfirmation), 200)
	_, e = f.ps.Archive(f.ctx, f.actors["lead"], f.team, f.project.ID, f.project.Version)
	must(t, e)
}

func TestServiceContributionScopeAndSessionBinding(t *testing.T) {
	f := setup(t)
	c := f.create()
	forged := f.actors["lead"]
	forged.UserID = f.actors["root"].UserID
	_, e := f.svc.Get(f.ctx, forged, f.team, f.project.ID, c)
	status(t, e, 401)
	_, e = f.svc.Get(f.ctx, f.actors["outsider"], f.team, f.project.ID, c)
	status(t, e, 404)
	other, e := f.ps.Create(f.ctx, f.actors["root"], f.team, projects.CreateInput{ProjectInput: projects.ProjectInput{Name: "Other project"}, LeadID: f.actors["lead"].UserID})
	must(t, e)
	_, e = f.svc.Create(f.ctx, f.actors["lead"], f.team, other.ID, contributions.CreateInput{TaskID: f.task.ID, ReviewID: f.review.ID, ClientRequestID: uuid.New(), Reason: "Cross project source"})
	status(t, e, 404)
	_, e = f.svc.VersionAction(context.Background(), reqctx.Identity{UserID: f.actors["lead"].UserID}, f.team, f.project.ID, c, "submit", f.current(c))
	status(t, e, 401)
	raw, e := json.Marshal(f.detail(c))
	must(t, e)
	if strings.Contains(string(raw), "proof_token") || strings.Contains(string(raw), "session_id") {
		t.Fatal("sensitive session implementation fields exposed")
	}
}
