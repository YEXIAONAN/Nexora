package bonusresults

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
	page    Page
	err     error
	calls   int
	team    uuid.UUID
	subject uuid.UUID
	session uuid.UUID
}

func (f *fakeReader) MyResults(_ context.Context, team, subject, session uuid.UUID) (Page, error) {
	f.calls++
	f.team, f.subject, f.session = team, subject, session
	return f.page, f.err
}

func requireCode(t *testing.T, err error, status int) {
	t.Helper()
	api, ok := err.(*httpx.APIError)
	if !ok || api.Status != status {
		t.Fatalf("error = %v; want HTTP %d", err, status)
	}
}

func TestMineUsesAuthenticatedSubjectOnly(t *testing.T) {
	team, user, session := uuid.New(), uuid.New(), uuid.New()
	round := uuid.New()
	f := &fakeReader{page: Page{Items: []Result{{
		RoundID: round, ProjectID: uuid.New(), CurrencyCode: "CNY",
		CurrencyMinorUnit: 2, AmountMinor: 100, ConfirmedAt: time.Now().UTC(),
	}}}}
	svc := NewService(f)
	got, err := svc.Mine(context.Background(), reqctx.Identity{UserID: user, SessionID: session, SystemRoot: true}, team)
	if err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 || f.team != team || f.subject != user || f.session != session {
		t.Fatal("results query did not use the authenticated subject and session")
	}
	if len(got.Items) != 1 || got.Items[0].RoundID != round || got.Items[0].AmountMinor != 100 {
		t.Fatalf("unexpected personal results: %+v", got)
	}
}

func TestMineFailsClosedOnMissingSessionOrMembership(t *testing.T) {
	team, user := uuid.New(), uuid.New()
	f := &fakeReader{}
	svc := NewService(f)
	_, err := svc.Mine(context.Background(), reqctx.Identity{UserID: user}, team)
	requireCode(t, err, http.StatusUnauthorized)
	if f.calls != 0 {
		t.Fatal("database query ran with a missing session")
	}
	_, err = svc.Mine(context.Background(), reqctx.Identity{UserID: user, SessionID: uuid.New()}, uuid.Nil)
	requireCode(t, err, http.StatusBadRequest)
	if f.calls != 0 {
		t.Fatal("invalid team scope queried the database")
	}
	f.err = ErrNotMember
	_, err = svc.Mine(context.Background(), reqctx.Identity{UserID: user, SessionID: uuid.New(), SystemRoot: true}, team)
	requireCode(t, err, http.StatusNotFound)
	if f.calls != 1 {
		t.Fatal("ROOT was incorrectly granted an implicit bonus visibility bypass")
	}
	f.err = errors.New("private database connection error")
	_, err = svc.Mine(context.Background(), reqctx.Identity{UserID: user, SessionID: uuid.New()}, team)
	requireCode(t, err, http.StatusInternalServerError)
}

func TestMineReturnsStableEmptyCollection(t *testing.T) {
	f := &fakeReader{page: Page{}}
	result, err := NewService(f).Mine(context.Background(), reqctx.Identity{UserID: uuid.New(), SessionID: uuid.New()}, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if result.Items == nil || result.HasMore {
		t.Fatal("empty result must be an explicit array with no next page")
	}
}
