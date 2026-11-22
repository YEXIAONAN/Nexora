package bonusrounds

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func validPlan(actor uuid.UUID) PlanInput {
	eligible := true
	return PlanInput{Command: Command{ExpectedRevision: 2, ClientRequestID: uuid.New(), Reason: "Manual allocation"}, TotalBudgetMinor: MaxAmountMinor,
		Sources: []PlanSourceInput{{ConfirmationID: uuid.New(), SourceBudgetMinor: MaxAmountMinor - 1, Reason: "Approved manual source allocation",
			Lines: []PlanLineInput{{UserID: uuid.New(), Eligible: &eligible, AmountMinor: MaxAmountMinor - 2, Reason: "Explicit eligibility and amount"}}}}}
}
func TestManualPlanValidation(t *testing.T) {
	actor := uuid.New()
	for _, tc := range []struct {
		name   string
		mutate func(*PlanInput)
	}{
		{"negative total", func(p *PlanInput) { p.TotalBudgetMinor = -1 }},
		{"over limit", func(p *PlanInput) { p.TotalBudgetMinor = MaxAmountMinor + 1 }},
		{"source exceeds total", func(p *PlanInput) { p.TotalBudgetMinor = 1 }},
		{"line exceeds source", func(p *PlanInput) { p.Sources[0].Lines[0].AmountMinor = MaxAmountMinor }},
		{"omitted eligibility", func(p *PlanInput) { p.Sources[0].Lines[0].Eligible = nil }},
		{"self eligibility", func(p *PlanInput) { p.Sources[0].Lines[0].UserID = actor }},
		{"ineligible amount", func(p *PlanInput) { no := false; p.Sources[0].Lines[0].Eligible = &no }},
		{"duplicate source", func(p *PlanInput) { p.Sources = append(p.Sources, p.Sources[0]) }},
		{"duplicate member", func(p *PlanInput) { p.Sources[0].Lines = append(p.Sources[0].Lines, p.Sources[0].Lines[0]) }},
		{"missing reason", func(p *PlanInput) { p.Sources[0].Reason = " " }},
		{"missing revision", func(p *PlanInput) { p.ExpectedRevision = 0 }},
		{"missing request", func(p *PlanInput) { p.ClientRequestID = uuid.Nil }},
		{"inconsistent eligibility", func(p *PlanInput) {
			no := false
			s := p.Sources[0]
			s.ConfirmationID = uuid.New()
			s.SourceBudgetMinor = 0
			s.Lines = []PlanLineInput{{UserID: s.Lines[0].UserID, Eligible: &no, Reason: "Not eligible"}}
			p.Sources = append(p.Sources, s)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := validPlan(actor)
			tc.mutate(&p)
			requireStatus(t, validateWorkflow("plan", p, actor), 400)
		})
	}
	if err := validateWorkflow("plan", validPlan(actor), actor); err != nil {
		t.Fatal(err)
	}
	// Unallocated funds at both levels are deliberate, not implicit redistribution.
	p := validPlan(actor)
	p.Sources[0].Lines[0].AmountMinor = 0
	if err := validateWorkflow("plan", p, actor); err != nil {
		t.Fatal(err)
	}
}
func TestCreateRequiresExplicitDistinctOfficers(t *testing.T) {
	actor := uuid.New()
	good := CreateInput{FundingKey: "school-2026", FundingLabel: "School", FundingSource: "School award", ManagerID: uuid.New(), ReviewerID: uuid.New(), Reason: "Explicit appointments", ClientRequestID: uuid.New()}
	if e := validateWorkflow("create", good, actor); e != nil {
		t.Fatal(e)
	}
	for _, officer := range []uuid.UUID{actor, good.ReviewerID, uuid.Nil} {
		bad := good
		bad.ManagerID = officer
		requireStatus(t, validateWorkflow("create", bad, actor), 400)
	}
}
func TestMoneyJSONRejectsFractionsAndOverflow(t *testing.T) {
	for _, value := range []string{"0.5", "1e3", "9223372036854775808", "\"100\""} {
		var p PlanInput
		if err := json.Unmarshal([]byte(`{"total_budget_minor":`+value+`}`), &p); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
}
func TestWorkflowErrorMapping(t *testing.T) {
	for code, want := range map[string]int{"P0002": 404, "42501": 403, "40001": 409, "23505": 409, "23514": 409, "22P02": 400, "22003": 400} {
		requireStatus(t, workflowError(&pgconn.PgError{Code: code}), want)
	}
}

type workflowFake struct {
	fakeReader
	calls  int
	action string
	input  any
}

func (f *workflowFake) ProjectWorkspace(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (ProjectWorkspace, error) {
	return ProjectWorkspace{}, f.err
}
func (f *workflowFake) Workspace(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (Workspace, error) {
	return Workspace{}, f.err
}
func (f *workflowFake) Execute(_ context.Context, team, project, round, user, session uuid.UUID, action string, input any) (CommandResult, error) {
	f.calls++
	f.team, f.project, f.round, f.subject, f.session = team, project, round, user, session
	f.action, f.input = action, input
	return CommandResult{RoundID: round, Revision: 3}, f.err
}

func TestWorkflowServiceAlwaysUsesSessionSubject(t *testing.T) {
	f := &workflowFake{}
	svc := NewService(f)
	team, project, round := uuid.New(), uuid.New(), uuid.New()
	a := reqctx.Identity{UserID: uuid.New(), SessionID: uuid.New(), SystemRoot: true}
	_, e := svc.Execute(context.Background(), a, team, project, round, "plan", validPlan(a.UserID))
	if e != nil {
		t.Fatal(e)
	}
	if f.subject != a.UserID || f.session != a.SessionID || f.team != team || f.project != project || f.round != round {
		t.Fatal("lost authenticated scope")
	}
	a.SessionID = uuid.Nil
	_, e = svc.Execute(context.Background(), a, team, project, round, "submit", Command{})
	requireStatus(t, e, 401)
	if f.calls != 1 {
		t.Fatal("unauthenticated command reached storage")
	}
}
func TestWorkflowRoutesRejectUnknownFieldsAndClientPaymentAmounts(t *testing.T) {
	f := &workflowFake{}
	h := NewHandler(NewService(f), slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := chi.NewRouter()
	h.AuthenticatedRoutes(r)
	team, project, round := uuid.New(), uuid.New(), uuid.New()
	base := "/teams/" + team.String() + "/projects/" + project.String() + "/bonus-rounds/" + round.String()
	for _, test := range []struct{ method, path, body string }{
		{"POST", base + "/payments", `{"expected_revision":2,"client_request_id":"` + uuid.NewString() + `","reason":"record","user_id":"` + uuid.NewString() + `","reference":"test","amount_minor":1}`},
		{"PUT", base + "/plan", `{"total_budget_minor":0.1}`},
		{"POST", base + "/confirm", `{"actor_id":"` + uuid.NewString() + `"}`},
	} {
		req := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(reqctx.WithIdentity(req.Context(), reqctx.Identity{UserID: uuid.New(), SessionID: uuid.New()}))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", test.path, rr.Code, rr.Body.String())
		}
	}
	if f.calls != 0 {
		t.Fatal("malformed input reached storage")
	}
}
