package bonusrounds

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

type fakeReader struct {
	page      Page
	detail    Detail
	err       error
	listCalls int
	getCalls  int
	team      uuid.UUID
	project   uuid.UUID
	round     uuid.UUID
	subject   uuid.UUID
	session   uuid.UUID
}

func (f *fakeReader) List(_ context.Context, team, project, user, session uuid.UUID) (Page, error) {
	f.listCalls++
	f.team, f.project, f.subject, f.session = team, project, user, session
	return f.page, f.err
}

func (f *fakeReader) Get(_ context.Context, team, project, round, user, session uuid.UUID) (Detail, error) {
	f.getCalls++
	f.team, f.project, f.round, f.subject, f.session = team, project, round, user, session
	return f.detail, f.err
}

func requireStatus(t *testing.T, err error, want int) {
	t.Helper()
	var api *httpx.APIError
	if !errors.As(err, &api) || api.Status != want {
		t.Fatalf("error = %v; want HTTP %d", err, want)
	}
}

func TestListUsesAuthenticatedSubjectAndHidesAmounts(t *testing.T) {
	team, project, user, session := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f := &fakeReader{page: Page{Items: []Summary{{
		ID: uuid.New(), ProjectID: project, FundingKey: "school-2026",
		FundingLabel: "School prize", FundingSource: "school", Status: StatusDraft,
		AmountsVisible: true, // must be forced back to false by the service
	}}}}
	svc := NewService(f)
	got, err := svc.List(context.Background(), reqctx.Identity{UserID: user, SessionID: session, SystemRoot: true}, team, project)
	if err != nil {
		t.Fatal(err)
	}
	if f.listCalls != 1 || f.team != team || f.project != project || f.subject != user || f.session != session {
		t.Fatal("list did not use the authenticated subject and scoped project")
	}
	if len(got.Items) != 1 || got.Items[0].AmountsVisible {
		t.Fatal("project read access exposed bonus amounts")
	}
}

func TestGetUsesAuthenticatedSubjectAndHidesAmounts(t *testing.T) {
	team, project, round, user, session := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f := &fakeReader{detail: Detail{
		Summary:  Summary{ID: round, ProjectID: project, AmountsVisible: true},
		Versions: []Version{{Version: 1, Status: "draft", Effective: false, CreatedAt: time.Now().UTC()}},
	}}
	svc := NewService(f)
	got, err := svc.Get(context.Background(), reqctx.Identity{UserID: user, SessionID: session, SystemRoot: true}, team, project, round)
	if err != nil {
		t.Fatal(err)
	}
	if f.getCalls != 1 || f.round != round || f.subject != user || f.session != session {
		t.Fatal("detail did not use the authenticated subject and scoped round")
	}
	if got.AmountsVisible {
		t.Fatal("detail exposed bonus amounts")
	}
}

func TestAccessFailuresFailClosed(t *testing.T) {
	team, project, round, user := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f := &fakeReader{}
	svc := NewService(f)
	_, err := svc.List(context.Background(), reqctx.Identity{UserID: user}, team, project)
	requireStatus(t, err, http.StatusUnauthorized)
	_, err = svc.Get(context.Background(), reqctx.Identity{UserID: user}, team, project, round)
	requireStatus(t, err, http.StatusUnauthorized)
	if f.listCalls != 0 || f.getCalls != 0 {
		t.Fatal("database was queried without an authenticated session")
	}

	_, err = svc.List(context.Background(), reqctx.Identity{UserID: user, SessionID: uuid.New()}, uuid.Nil, project)
	requireStatus(t, err, http.StatusBadRequest)
	_, err = svc.Get(context.Background(), reqctx.Identity{UserID: user, SessionID: uuid.New()}, team, project, uuid.Nil)
	requireStatus(t, err, http.StatusBadRequest)
	if f.listCalls != 0 || f.getCalls != 0 {
		t.Fatal("invalid scope queried the database")
	}

	f.err = ErrNotFound
	_, err = svc.List(context.Background(), reqctx.Identity{UserID: user, SessionID: uuid.New(), SystemRoot: true}, team, project)
	requireStatus(t, err, http.StatusNotFound)
	_, err = svc.Get(context.Background(), reqctx.Identity{UserID: user, SessionID: uuid.New(), SystemRoot: true}, team, project, round)
	requireStatus(t, err, http.StatusNotFound)
	if f.listCalls != 1 || f.getCalls != 1 {
		t.Fatal("ROOT unexpectedly bypassed bonus visibility")
	}

	f.err = errors.New("private database detail")
	_, err = svc.List(context.Background(), reqctx.Identity{UserID: user, SessionID: uuid.New()}, team, project)
	requireStatus(t, err, http.StatusInternalServerError)
}

func TestEmptyResponsesAreExplicitCollections(t *testing.T) {
	f := &fakeReader{}
	svc := NewService(f)
	page, err := svc.List(context.Background(), reqctx.Identity{UserID: uuid.New(), SessionID: uuid.New()}, uuid.New(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if page.Items == nil || page.HasMore {
		t.Fatal("empty list must be an explicit array without a next page")
	}
	detail, err := svc.Get(context.Background(), reqctx.Identity{UserID: uuid.New(), SessionID: uuid.New()}, uuid.New(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if detail.Versions == nil {
		t.Fatal("empty version history must be an explicit array")
	}
}

func TestDeriveStatus(t *testing.T) {
	draft := &Version{Version: 1, Status: "draft"}
	pending := &Version{Version: 1, Status: "pending_review"}
	confirmed := &Version{Version: 1, Status: "confirmed", ConfirmedAt: ptrTime(time.Now().UTC())}
	newerDraft := &Version{Version: 2, Status: "draft"}
	abandoned := &Version{Version: 1, Status: "abandoned"}
	for _, tc := range []struct {
		name      string
		latest    *Version
		effective *Version
		want      string
	}{
		{"no version", nil, nil, StatusDraft},
		{"draft", draft, nil, StatusDraft},
		{"pending review", pending, nil, StatusPendingReview},
		{"confirmed", confirmed, confirmed, StatusConfirmed},
		{"newer draft", newerDraft, confirmed, StatusRevising},
		{"newer abandoned revision", &Version{Version: 2, Status: "abandoned"}, confirmed, StatusConfirmed},
		{"abandoned", abandoned, nil, StatusAbandoned},
		{"confirmed without pointer", confirmed, nil, StatusUnavailable},
		{"newer confirmed without moved pointer", &Version{Version: 2, Status: "confirmed"}, confirmed, StatusUnavailable},
		{"superseded without pointer", &Version{Version: 3, Status: "superseded"}, nil, StatusUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveStatus(tc.latest, tc.effective); got != tc.want {
				t.Fatalf("status = %q, want %q", got, tc.want)
			}
		})
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
