//go:build integration

package contributions_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/auth"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/contributions"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/submissions"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/tasks"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/teams"
)

func TestHTTPContributionLifecycle(t *testing.T) {
	f := setup(t)
	f.grant("appraiser")
	f.grant("arbiter")
	c := f.create()
	draft := f.current(c)
	f.http("PATCH", f.path(c, "/draft"), "lead", contributions.DraftInput{VersionCommand: draft, Summary: "Manual contribution explanation", EvidenceSnapshot: map[string]any{"source": "accepted task", "limitation": "behavior is not a contribution percentage"}}, 200)
	evidence := data[contributions.AcceptanceEvidence](t, f.do("GET", f.path(c, "/evidence"), nil, f.cookies["lead"], false), 200)
	if evidence.ReviewID != f.review.ID || evidence.SubmissionVersionID != f.submission.ID || evidence.Decision != "approve" || len(evidence.Attachments) != 1 {
		t.Fatal("case did not bind real immutable acceptance")
	}
	f.share(c, 4000)
	if d := f.detail(c); d.Latest.TotalBPS != 4000 || d.Latest.ReadyToSubmit || d.Effective != nil {
		t.Fatal("partial draft validation or effective read is wrong")
	}
	data[any](t, f.do("POST", f.path(c, "/submit"), f.current(c), f.cookies["lead"], true), 409)
	f.share(c, 10000)
	// Explicit removal preserves append-only adjustments.
	who := f.actors["member"].UserID
	f.http("PUT", f.path(c, "/shares/"+who.String()), "lead", contributions.ShareInput{VersionCommand: f.current(c), ShareBPS: 1, Basis: "Temporary proposal", EvidenceRefs: []contributions.EvidenceReference{}}, 200)
	f.http("DELETE", f.path(c, "/shares/"+who.String()), "lead", f.current(c), 200)
	if !f.detail(c).Latest.ReadyToSubmit {
		t.Fatal("complete draft was not ready")
	}
	f.http("POST", f.path(c, "/submit"), "lead", f.current(c), 200)
	pending := data[[]contributions.Case](t, f.do("GET", f.base()+"/contributions/pending", nil, f.cookies["appraiser"], false), 200)
	if len(pending) != 1 {
		t.Fatal("independent appraisal inbox omitted a pending case")
	}
	data[any](t, f.do("POST", f.path(c, "/withdraw"), f.current(c), f.cookies["appraiser"], true), 403)
	f.http("POST", f.path(c, "/withdraw"), "lead", f.current(c), 200)
	f.http("POST", f.path(c, "/submit"), "lead", f.current(c), 200)
	data[any](t, f.do("POST", f.path(c, "/confirm"), f.current(c), f.cookies["lead"], true), 403)
	confirmation := f.confirm(c)
	d := f.detail(c)
	old := d.Latest.ID
	if d.Effective == nil || !d.Case.UndisputedConfirmed || d.Effective.Confirmation.ID != confirmation || f.official(c) != 1 {
		t.Fatal("confirmed result not effective")
	}
	data[any](t, f.do("PUT", f.path(c, "/shares/"+f.actors["worker"].UserID.String()), contributions.ShareInput{VersionCommand: f.current(c), ShareBPS: 5000, EvidenceRefs: []contributions.EvidenceReference{}}, f.cookies["lead"], true), 409)
	f.http("POST", f.path(c, "/revisions"), "lead", f.current(c), 200)
	d = f.detail(c)
	if d.Latest.ID == old || d.Effective.ID != old || f.official(c) != 1 || d.Case.CaseWorkflowStatus != "revising" {
		t.Fatal("draft replaced the effective confirmation")
	}
	f.prepare(c)
	x := f.current(c)
	x.CaseVersionID = old
	dispute := f.http("POST", f.path(c, "/disputes"), "worker", contributions.DisputeInput{VersionCommand: x, ConfirmationID: confirmation}, 201).ID
	if f.official(c) != 0 {
		t.Fatal("disputed result appeared in official shares")
	}

	f.http("POST", f.path(c, "/withdraw"), "lead", f.current(c), 200)
	blocked := f.detail(c)
	if !blocked.Latest.ReadyToSubmit || blocked.Permissions.CanSubmit {
		t.Fatal("disputed case offered a submit action forbidden by v7")
	}
	data[any](t, f.do("POST", f.path(c, "/submit"), f.current(c), f.cookies["lead"], true), 409)
	data[any](t, f.do("POST", f.path(c, "/disputes/"+dispute.String()+"/resolutions"), contributions.ResolutionInput{Command: f.current(c).Command, Decision: "uphold"}, f.cookies["appraiser"], true), 403)
	f.http("POST", f.path(c, "/disputes/"+dispute.String()+"/resolutions"), "arbiter", contributions.ResolutionInput{Command: f.current(c).Command, Decision: "uphold"}, 200)
	if f.official(c) != 1 {
		t.Fatal("uphold changed the allocation")
	}
	f.http("POST", f.path(c, "/submit"), "lead", f.current(c), 200)
	x = f.current(c)
	x.CaseVersionID = old
	dispute = f.http("POST", f.path(c, "/disputes"), "worker", contributions.DisputeInput{VersionCommand: x, ConfirmationID: confirmation}, 201).ID
	f.http("POST", f.path(c, "/disputes/"+dispute.String()+"/resolutions"), "arbiter", contributions.ResolutionInput{Command: f.current(c).Command, Decision: "overturn"}, 200)
	d = f.detail(c)
	if d.Effective != nil || d.Latest.Status != "draft" || f.official(c) != 0 {
		t.Fatal("overturn did not clear effectiveness atomically")
	}
	hist := data[contributions.History](t, f.do("GET", f.path(c, "/history"), nil, f.cookies["lead"], false), 200)
	if len(hist.Versions) != 3 || len(hist.Disputes) != 2 || hist.Versions[0].Status != "overturned" || hist.Versions[0].Confirmation.ID != confirmation || hist.Versions[1].Status != "abandoned" || len(hist.Adjustments) < 4 {
		t.Fatal("immutable history lost during overturn")
	}
	f.prepare(c)
	confirmation = f.confirm(c)
	notice := f.notice(c, confirmation)
	results := data[[]contributions.Result](t, f.do("GET", f.base()+"/contribution-results/mine", nil, f.cookies["worker"], false), 200)
	if len(results) != 1 || results[0].Notice.Receipt != nil || results[0].Confirmation.ID != confirmation {
		t.Fatal("GET inferred delivery or omitted the confirmed result")
	}
	_, e := f.ps.Archive(f.ctx, f.actors["lead"], f.team, f.project.ID, f.project.Version)
	status(t, e, 409)
	in := f.receiptInput(c, confirmation)
	data[any](t, f.do("POST", f.path(c, "/notices/"+notice.String()+"/receipt"), in, f.cookies["appraiser"], true), 403)
	bad := in
	bad.ContentDigest = strings.Repeat("0", 64)
	data[any](t, f.do("POST", f.path(c, "/notices/"+notice.String()+"/receipt"), bad, f.cookies["worker"], true), 409)
	bad = in
	bad.Received = false
	data[any](t, f.do("POST", f.path(c, "/notices/"+notice.String()+"/receipt"), bad, f.cookies["worker"], true), 422)
	body, _ := json.Marshal(in)
	request := httptest.NewRequest("POST", f.path(c, "/notices/"+notice.String()+"/receipt"), bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(auth.CSRFHeader, "1")
	request.Header.Set("X-Request-Id", f.cookies["worker"].Value)
	request.AddCookie(f.cookies["worker"])
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	if response.Header().Get("X-Request-Id") == f.cookies["worker"].Value {
		t.Fatal("session credential reflected as request ID")
	}
	receipt := data[contributions.CommandResult](t, response, 200)
	retry := f.http("POST", f.path(c, "/notices/"+notice.String()+"/receipt"), "worker", in, 200)
	if !reflect.DeepEqual(receipt, retry) {
		t.Fatal("receipt replay did not return the original command result")
	}
	var verified bool
	must(t, f.db.Pool.QueryRow(f.ctx, `SELECT proof_token IS NULL AND session_id=$2 AND subject_id=$3 AND acknowledged_at IS NOT NULL FROM contribution_attestations WHERE id=$1`, receipt.ID, f.actors["worker"].SessionID, f.actors["worker"].UserID).Scan(&verified))
	if !verified {
		t.Fatal("receipt was not bound to the real authenticated cookie")
	}
	versions := data[[]contributions.Version](t, f.do("GET", f.path(c, "/versions"), nil, f.cookies["worker"], false), 200)
	if len(versions) != 3 {
		t.Fatal("related member cannot inspect revision history")
	}
	_, e = f.ps.Archive(f.ctx, f.actors["lead"], f.team, f.project.ID, f.project.Version)
	must(t, e)
	archivedEligibility := data[contributions.Eligibility](t, f.do("GET", f.base()+"/contributions/eligibility", nil, f.cookies["lead"], false), 200)
	if archivedEligibility.CanEditCases {
		t.Fatal("archived project offered contribution editing")
	}
	data[any](t, f.do("POST", f.path(c, "/revisions"), f.current(c), f.cookies["root"], true), 404)
	data[any](t, f.do("POST", f.path(c, "/revisions"), f.current(c), f.cookies["member"], true), 404)
	data[any](t, f.do("POST", f.path(c, "/revisions"), f.current(c), f.cookies["lead"], true), 409)
	f.noCredentials()
}
func TestHTTPContributionIdentityAuthorizationAndValidation(t *testing.T) {
	f := setup(t)
	c := f.create()
	for _, who := range []string{"root", "outsider"} {
		data[any](t, f.do("GET", f.path(c, ""), nil, f.cookies[who], false), 404)
	}
	data[any](t, f.do("GET", f.path(c, ""), nil, nil, false), 401)
	data[any](t, f.do("POST", f.path(c, "/submit"), f.current(c), f.cookies["lead"], false), 403)
	data[any](t, f.do("GET", f.path(c, ""), nil, f.cookies["member"], false), 404)
	list := data[[]contributions.Case](t, f.do("GET", f.base()+"/contributions", nil, f.cookies["member"], false), 200)
	if len(list) != 0 {
		t.Fatal("unrelated member can enumerate private cases")
	}
	data[any](t, f.do("PATCH", f.path(c, "/draft"), contributions.DraftInput{VersionCommand: f.current(c), EvidenceSnapshot: map[string]any{}}, f.cookies["member"], true), 404)
	// Platform admin privilege does not create team/project membership.
	must(t, f.as.GrantPlatformRole(f.ctx, f.actors["root"], f.actors["outsider"].UserID))
	f.login("outsider")
	data[any](t, f.do("GET", f.path(c, ""), nil, f.cookies["outsider"], false), 404)
	for _, field := range []string{"actor_id", "confirmed_by", "proof_token", "effective_confirmed_version_id", "case_workflow_status"} {
		in := map[string]any{"expected_version": f.current(c).ExpectedVersion, "case_version_id": f.current(c).CaseVersionID, "client_request_id": uuid.New(), "reason": "client cannot choose the operator", field: f.actors["root"].UserID}
		data[any](t, f.do("POST", f.path(c, "/confirm"), in, f.cookies["lead"], true), 400)
	}
	missing := map[string]any{"user_id": f.actors["appraiser"].UserID, "client_request_id": uuid.New(), "reason": "explicit version required"}
	data[any](t, f.do("POST", f.base()+"/contribution-appraisers", missing, f.cookies["lead"], true), 422)
	data[any](t, f.do("POST", f.base()+"/contribution-appraisers", contributions.AppraiserInput{Command: contributions.Command{ExpectedVersion: 0, ClientRequestID: uuid.New(), Reason: "self grant"}, UserID: f.actors["lead"].UserID}, f.cookies["lead"], true), 403)
	forged := contributions.CreateInput{TaskID: f.task.ID, ReviewID: uuid.New(), ClientRequestID: uuid.New(), Reason: "fake review"}
	data[any](t, f.do("POST", f.base()+"/contributions", forged, f.cookies["lead"], true), 404)
	other, e := f.ps.Create(f.ctx, f.actors["root"], f.team, projects.CreateInput{ProjectInput: projects.ProjectInput{Name: "Other scope"}, LeadID: f.actors["lead"].UserID})
	must(t, e)
	cross := strings.Replace(f.path(c, ""), f.project.ID.String(), other.ID.String(), 1)
	data[any](t, f.do("GET", cross, nil, f.cookies["lead"], false), 404)
	f.grant("appraiser")
	f.grant("worker")
	f.prepare(c)
	spoofedBody, _ := json.Marshal(f.current(c))
	request := httptest.NewRequest("POST", f.path(c, "/confirm")+"?actor_id="+f.actors["appraiser"].UserID.String(), bytes.NewReader(spoofedBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(auth.CSRFHeader, "1")
	request.Header.Set("X-Nexora-Actor-ID", f.actors["appraiser"].UserID.String())
	request.AddCookie(f.cookies["worker"])
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	data[any](t, response, 403)

	data[any](t, f.do("POST", f.path(c, "/confirm"), f.current(c), f.cookies["worker"], true), 403)
	data[any](t, f.do("POST", f.path(c, "/confirm"), f.current(c), f.cookies["lead"], true), 403)
	f.confirm(c)
	data[any](t, f.do("PUT", f.path(c, "/shares/"+f.actors["worker"].UserID.String()), map[string]any{"expected_version": 1, "client_request_id": uuid.New(), "reason": "float rejected", "case_version_id": f.current(c).CaseVersionID, "share_bps": 0.5, "basis": "invalid", "statement": "", "evidence_refs": []any{}}, f.cookies["lead"], true), 400)
	// A second JSON object cannot smuggle an actor or extra command.
	payload, _ := json.Marshal(f.current(c))
	raw := append(payload, []byte(` {"actor_id":"forged"}`)...)
	req := httptest.NewRequest("POST", f.path(c, "/confirm"), bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.CSRFHeader, "1")
	req.AddCookie(f.cookies["appraiser"])
	rr := httptest.NewRecorder()
	f.handler.ServeHTTP(rr, req)
	data[any](t, rr, 400)
}
func TestHTTPContributionSessionFailures(t *testing.T) {
	f := setup(t)
	c := f.create()
	path := f.path(c, "")
	data[any](t, f.do("GET", path, nil, &http.Cookie{Name: auth.SessionCookieName, Value: "not-a-session"}, false), 401)
	_, e := f.db.Pool.Exec(f.ctx, `UPDATE sessions SET created_at=clock_timestamp()-interval '2 hours',expires_at=clock_timestamp()-interval '1 hour' WHERE id=$1`, f.actors["lead"].SessionID)
	must(t, e)
	data[any](t, f.do("GET", path, nil, f.cookies["lead"], false), 401)
	f.login("lead")
	data[any](t, f.do("DELETE", "/api/v1/auth/session", nil, f.cookies["lead"], true), 200)
	data[any](t, f.do("GET", path, nil, f.cookies["lead"], false), 401)

}
func (f *fixture) noCredentials() {
	f.t.Helper()
	for _, cookie := range f.cookies {
		if strings.Contains(f.logs.String(), cookie.Value) {
			f.t.Fatal("log exposed a raw session credential")
		}
		for _, table := range []string{"contribution_attestations", "contribution_notifications", "contribution_case_events", "audit_events", "sessions"} {
			var found bool
			must(f.t, f.db.Pool.QueryRow(f.ctx, `SELECT position($1 in COALESCE(jsonb_agg(to_jsonb(t))::text,''))>0 FROM `+table+` t`, cookie.Value).Scan(&found))
			if found {
				f.t.Fatal("persisted raw session credential in", table)
			}
		}
	}
}

func TestHTTPContributionDepartedSubjectProxyRequest(t *testing.T) {
	f := setup(t)
	f.grant("appraiser")
	f.grant("arbiter")
	c := f.create()
	f.prepare(c)
	conf := f.confirm(c)
	version := f.detail(c).Latest.ID
	// The subject can see their own related case before departure.
	data[contributions.CaseDetail](t, f.do("GET", f.path(c, ""), nil, f.cookies["worker"], false), 200)
	must(t, f.ts.RemoveMember(f.ctx, f.actors["root"], f.team, f.actors["worker"].UserID))
	for _, path := range []string{f.path(c, ""), f.path(c, "/history"), f.base() + "/contribution-results/mine"} {
		data[any](t, f.do("GET", path, nil, f.cookies["worker"], false), 404)
	}
	in := contributions.IdentityRequestInput{ClientRequestID: uuid.New(), TeamID: f.team, ProjectID: f.project.ID, CaseID: c, CaseVersionID: version, ConfirmationID: conf, Reason: "Please review my historical allocation"}
	for i := 0; i < 2; i++ {
		out := data[contributions.Intake](t, f.do("POST", "/api/v1/contribution-dispute-requests", in, f.cookies["worker"], true), 202)
		if out.RequestID != in.ClientRequestID || !out.Received {
			t.Fatal("intake must return only the caller's receipt")
		}
	}
	var verified bool
	must(t, f.db.Pool.QueryRow(f.ctx, `SELECT subject_id=$2 AND actor_id=$2 AND session_id=$3 AND proof_token IS NULL AND purpose='dispute_request' FROM contribution_attestations WHERE id=$1`, in.ClientRequestID, f.actors["worker"].UserID, f.actors["worker"].SessionID).Scan(&verified))
	if !verified {
		t.Fatal("historical subject request lacked actual cookie proof")
	}
	proofPath := f.path(c, "/dispute-requests/"+in.ClientRequestID.String())
	proof := data[contributions.IdentityRequestEvidence](t, f.do("GET", proofPath, nil, f.cookies["lead"], false), 200)
	if proof.SubjectID != f.actors["worker"].UserID || proof.Statement != in.Reason || proof.ConfirmationID != conf || proof.VerifiedAt.IsZero() {
		t.Fatal("authorized proxy operator cannot verify the actual subject request")
	}
	data[any](t, f.do("GET", proofPath, nil, f.cookies["worker"], false), 404)
	data[any](t, f.do("GET", f.path(c, "/dispute-requests/"+uuid.NewString()), nil, f.cookies["lead"], false), 404)

	invalid := in
	invalid.ClientRequestID = uuid.New()
	invalid.TeamID = uuid.New()
	reply := f.do("POST", "/api/v1/contribution-dispute-requests", invalid, f.cookies["worker"], true)
	data[contributions.Intake](t, reply, 202)
	var n int
	must(t, f.db.Pool.QueryRow(f.ctx, `SELECT count(*) FROM contribution_attestations WHERE id=$1`, invalid.ClientRequestID).Scan(&n))
	if n != 0 {
		t.Fatal("invalid intake manufactured evidence")
	}
	outsider := in
	outsider.ClientRequestID = uuid.New()
	data[contributions.Intake](t, f.do("POST", "/api/v1/contribution-dispute-requests", outsider, f.cookies["outsider"], true), 202)
	must(t, f.db.Pool.QueryRow(f.ctx, `SELECT count(*) FROM contribution_attestations WHERE id=$1`, outsider.ClientRequestID).Scan(&n))
	if n != 0 {
		t.Fatal("outsider obtained a historical identity credential")
	}
	worker := f.actors["worker"].UserID
	cmd := f.current(c)
	cmd.Reason = in.Reason
	proxy := contributions.DisputeInput{VersionCommand: cmd, ConfirmationID: conf, OnBehalfOf: &worker, IdentityRequestID: &in.ClientRequestID}
	wrong := proxy
	wrong.Reason = "an unverified different statement"
	wrong.ClientRequestID = uuid.New()
	data[any](t, f.do("POST", f.path(c, "/disputes"), wrong, f.cookies["lead"], true), 404)
	d := f.http("POST", f.path(c, "/disputes"), "lead", proxy, 201)
	ds := data[[]contributions.Dispute](t, f.do("GET", f.path(c, "/disputes"), nil, f.cookies["arbiter"], false), 200)
	if len(ds) != 1 || ds[0].OnBehalfOf == nil || *ds[0].OnBehalfOf != worker || ds[0].IdentityRequestEvidence == nil || ds[0].IdentityRequestEvidence.Statement != in.Reason || ds[0].VerificationMethod != "authenticated_request" || !ds[0].CanResolve || f.official(c) != 0 || f.detail(c).Case.UndisputedConfirmed {
		t.Fatal("proxy dispute identity, authority or official exclusion is wrong")
	}
	f.http("POST", f.path(c, "/disputes/"+d.ID.String()+"/resolutions"), "arbiter", contributions.ResolutionInput{Command: f.current(c).Command, Decision: "uphold"}, 200)
	f.noCredentials()
}

func TestHTTPContributionRejectedReviewAndCrossTeamIDOR(t *testing.T) {
	f := setup(t)
	task, e := f.tasks.Create(f.ctx, f.actors["lead"], f.team, f.project.ID, tasks.CreateInput{TaskFields: tasks.TaskFields{Title: "Rejected then accepted", OwnerID: f.actors["worker"].UserID}, Weight: 1})
	must(t, e)
	task, e = f.tasks.Transition(f.ctx, f.actors["worker"], f.team, f.project.ID, task.ID, tasks.InProgress, "", task.Version)
	must(t, e)
	first, e := f.sub.Submit(f.ctx, f.actors["worker"], f.team, f.project.ID, task.ID, submissions.SubmitInput{ClientRequestID: uuid.New(), ExpectedVersion: task.Version, Summary: "First actual submission"})
	must(t, e)
	task, e = f.tasks.Get(f.ctx, f.actors["lead"], f.team, f.project.ID, task.ID)
	must(t, e)
	rejected, e := f.sub.Review(f.ctx, f.actors["lead"], f.team, f.project.ID, task.ID, first.ID, submissions.ReviewInput{ExpectedVersion: task.Version, Decision: "reject", Comment: "Correct this evidence"})
	must(t, e)
	in := contributions.CreateInput{TaskID: task.ID, ReviewID: rejected.ID, ClientRequestID: uuid.New(), Reason: "Rejected evidence cannot create a case"}
	data[any](t, f.do("POST", f.base()+"/contributions", in, f.cookies["lead"], true), 404)
	task, e = f.tasks.Get(f.ctx, f.actors["worker"], f.team, f.project.ID, task.ID)
	must(t, e)
	second, e := f.sub.Submit(f.ctx, f.actors["worker"], f.team, f.project.ID, task.ID, submissions.SubmitInput{ClientRequestID: uuid.New(), ExpectedVersion: task.Version, Summary: "Corrected V2"})
	must(t, e)
	task, e = f.tasks.Get(f.ctx, f.actors["lead"], f.team, f.project.ID, task.ID)
	must(t, e)
	accepted, e := f.sub.Review(f.ctx, f.actors["lead"], f.team, f.project.ID, task.ID, second.ID, submissions.ReviewInput{ExpectedVersion: task.Version, Decision: "approve", Comment: "Accept current V2"})
	must(t, e)
	in.ClientRequestID = uuid.New()
	data[any](t, f.do("POST", f.base()+"/contributions", in, f.cookies["lead"], true), 404)
	in.ReviewID = accepted.ID
	in.ClientRequestID = uuid.New()
	result := f.http("POST", f.base()+"/contributions", "lead", in, 201)
	// The attacking user has a real valid relationship in the URL's other Team
	// and Project. UUID substitution must still not reach this accepted source.
	other, e := f.ts.CreateTeam(f.ctx, f.actors["root"], "Other real team", "")
	must(t, e)
	invitation, e := f.ts.Invite(f.ctx, f.actors["root"], other.ID, "outsider", teams.RoleMember, 1)
	must(t, e)
	_, e = f.ts.AcceptInvitation(f.ctx, f.actors["outsider"], invitation.ID)
	must(t, e)
	p, e := f.ps.Create(f.ctx, f.actors["root"], other.ID, projects.CreateInput{ProjectInput: projects.ProjectInput{Name: "Other real project"}, LeadID: f.actors["outsider"].UserID})
	must(t, e)
	path := fmt.Sprintf("/api/v1/teams/%s/projects/%s/contributions", other.ID, p.ID)
	data[any](t, f.do("GET", path+"/"+result.ID.String(), nil, f.cookies["outsider"], false), 404)
	in.ClientRequestID = uuid.New()
	data[any](t, f.do("POST", path, in, f.cookies["outsider"], true), 404)
	list := data[[]contributions.Case](t, f.do("GET", path, nil, f.cookies["outsider"], false), 200)
	if len(list) != 0 {
		t.Fatal("cross-team case was exposed or created")
	}
}
