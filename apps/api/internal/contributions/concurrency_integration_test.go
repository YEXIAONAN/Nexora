//go:build integration

package contributions_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/contributions"
)

// Both calls use independent service transactions/connections. A third
// connection observes real PostgreSQL lock waits for this isolated schema.
// The first caller is queued before the second, then the blocking Team lock
// is released. No mocks, sleep-only assumptions or synthetic lock outcomes.
func (f *fixture) pair(first, second func() error) (error, error) {
	f.t.Helper()
	block, e := f.db.SQL.BeginTx(f.ctx, nil)
	must(f.t, e)
	defer block.Rollback()
	_, e = block.ExecContext(f.ctx, `SELECT id FROM teams WHERE id=$1 FOR UPDATE`, f.team)
	must(f.t, e)
	monitor, e := pgx.ConnectConfig(f.ctx, f.db.Pool.Config().ConnConfig.Copy())
	must(f.t, e)
	defer monitor.Close(context.Background())
	a, b := make(chan error, 1), make(chan error, 1)
	wait := func(want int) {
		f.t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			var n int
			e := monitor.QueryRow(f.ctx, `SELECT count(*) FROM pg_stat_activity a WHERE a.datname=current_database() AND a.wait_event_type='Lock' AND EXISTS(SELECT 1 FROM pg_locks l WHERE l.pid=a.pid AND l.relation='teams'::regclass)`).Scan(&n)
			must(f.t, e)
			if n >= want {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		f.t.Fatal("service callers did not actually contend on the Team lock")
	}
	go func() { a <- first() }()
	wait(1)
	go func() { b <- second() }()
	wait(2)
	must(f.t, block.Commit())
	receive := func(ch <-chan error) error {
		select {
		case e := <-ch:
			var state interface{ SQLState() string }
			if errors.As(e, &state) && state.SQLState() == "40P01" {
				f.t.Fatal("PostgreSQL deadlock")
			}
			return e
		case <-time.After(10 * time.Second):
			f.t.Fatal("service transaction did not finish")
			return nil
		}
	}
	return receive(a), receive(b)
}
func TestServiceConcurrentContributionCreationAndReplay(t *testing.T) {
	for _, sameKey := range []bool{false, true} {
		t.Run(map[bool]string{false: "review_unique", true: "same_command"}[sameKey], func(t *testing.T) {
			f := setup(t)
			x := contributions.CreateInput{TaskID: f.task.ID, ReviewID: f.review.ID, ClientRequestID: uuid.New(), Reason: "One accepted review"}
			y := x
			if !sameKey {
				y.ClientRequestID = uuid.New()
			}
			var one, two contributions.CommandResult
			a, b := f.pair(func() error {
				var e error
				one, e = f.svc.Create(f.ctx, f.actors["lead"], f.team, f.project.ID, x)
				return e
			}, func() error {
				var e error
				two, e = f.svc.Create(f.ctx, f.actors["lead"], f.team, f.project.ID, y)
				return e
			})
			must(t, a)
			must(t, b)
			if one.ID != two.ID || sameKey && !reflect.DeepEqual(one, two) {
				t.Fatal("concurrent retry created a duplicate or changed replay")
			}
			var n int
			must(t, f.db.Pool.QueryRow(f.ctx, `SELECT count(*) FROM contribution_case_versions WHERE case_id=$1`, one.ID).Scan(&n))
			if n != 1 {
				t.Fatal("duplicate initial version")
			}
			if sameKey {
				must(t, f.db.Pool.QueryRow(f.ctx, `SELECT count(*) FROM audit_events WHERE action='contribution.command_completed' AND details->>'client_request_id'=$1`, x.ClientRequestID.String()).Scan(&n))
				if n != 1 {
					t.Fatal("concurrent replay duplicated command receipt")
				}
			}
		})
	}
}
func TestServiceConcurrentSharesAndDoubleConfirmation(t *testing.T) {
	f := setup(t)
	f.grant("appraiser")
	f.grant("arbiter")
	c := f.create()
	in := contributions.ShareInput{VersionCommand: f.current(c), ShareBPS: 7000, Basis: "Manual proposal", EvidenceRefs: []contributions.EvidenceReference{}}
	other := in
	other.ShareBPS = 8000
	other.ClientRequestID = uuid.New()
	a, b := f.pair(func() error {
		_, e := f.svc.SetShare(f.ctx, f.actors["lead"], f.team, f.project.ID, c, f.actors["worker"].UserID, in, false)
		return e
	}, func() error {
		_, e := f.svc.SetShare(f.ctx, f.actors["lead"], f.team, f.project.ID, c, f.actors["worker"].UserID, other, false)
		return e
	})
	must(t, a)
	status(t, b, 409)
	if f.detail(c).Latest.TotalBPS != 7000 {
		t.Fatal("concurrent allocation lost an update")
	}
	f.prepare(c)
	x, y := f.current(c), f.current(c)
	a, b = f.pair(func() error {
		_, e := f.svc.VersionAction(f.ctx, f.actors["appraiser"], f.team, f.project.ID, c, "confirm", x)
		return e
	}, func() error {
		_, e := f.svc.VersionAction(f.ctx, f.actors["arbiter"], f.team, f.project.ID, c, "confirm", y)
		return e
	})
	must(t, a)
	status(t, b, 409)
	var n int
	must(t, f.db.Pool.QueryRow(f.ctx, `SELECT count(*) FROM contribution_confirmations WHERE case_id=$1`, c).Scan(&n))
	if n != 1 {
		t.Fatal("double confirmation through Service")
	}
}
func TestServiceConcurrentConfirmationAndAuthorizationExit(t *testing.T) {
	for _, kind := range []string{"revoke", "project_exit", "team_exit"} {
		for _, confirmFirst := range []bool{false, true} {
			name := kind + map[bool]string{false: "_change_first", true: "_confirm_first"}[confirmFirst]
			t.Run(name, func(t *testing.T) {
				f := setup(t)
				f.grant("appraiser")
				c := f.create()
				f.prepare(c)
				in := f.current(c)
				confirm := func() error {
					_, e := f.svc.VersionAction(f.ctx, f.actors["appraiser"], f.team, f.project.ID, c, "confirm", in)
					return e
				}
				change := func() error {
					switch kind {
					case "revoke":
						_, e := f.svc.SetAppraiser(f.ctx, f.actors["lead"], f.team, f.project.ID, f.actors["appraiser"].UserID, contributions.Command{ExpectedVersion: 1, ClientRequestID: uuid.New(), Reason: "Revoke during appraisal"}, true)
						return e
					case "project_exit":
						_, e := f.ps.ChangeMember(f.ctx, f.actors["lead"], f.team, f.project.ID, f.actors["appraiser"].UserID, false)
						return e
					default:
						return f.ts.RemoveMember(f.ctx, f.actors["root"], f.team, f.actors["appraiser"].UserID)
					}
				}
				first, second := change, confirm
				if confirmFirst {
					first, second = second, first
				}
				a, b := f.pair(first, second)
				must(t, a)
				if confirmFirst {
					must(t, b)
					if f.official(c) != 1 {
						t.Fatal("later revocation changed a historical valid confirmation")
					}
				} else {
					status(t, b, 404)
					if f.official(c) != 0 || f.detail(c).Latest.Status != "pending_review" {
						t.Fatal("unqualified appraiser was accepted")
					}
				}
			})
		}
	}
}
func TestServiceConcurrentConfirmationDisputeAndRuling(t *testing.T) {
	for _, disputeFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirmation_first", true: "dispute_first"}[disputeFirst], func(t *testing.T) {
			f := setup(t)
			f.grant("appraiser")
			f.grant("arbiter")
			c := f.create()
			f.prepare(c)
			conf := f.confirm(c)
			old := f.current(c).CaseVersionID
			f.http("POST", f.path(c, "/revisions"), "lead", f.current(c), 200)
			f.prepare(c)
			x := f.current(c)
			di := contributions.DisputeInput{VersionCommand: f.current(c), ConfirmationID: conf}
			di.CaseVersionID = old
			confirm := func() error {
				_, e := f.svc.VersionAction(f.ctx, f.actors["appraiser"], f.team, f.project.ID, c, "confirm", x)
				return e
			}
			dispute := func() error {
				_, e := f.svc.RaiseDispute(f.ctx, f.actors["worker"], f.team, f.project.ID, c, di)
				return e
			}
			first, second := confirm, dispute
			if disputeFirst {
				first, second = second, first
			}
			a, b := f.pair(first, second)
			must(t, a)
			status(t, b, 409)
			if disputeFirst {
				if f.official(c) != 0 || f.detail(c).Case.UndisputedConfirmed {
					t.Fatal("disputed result counted as undisputed")
				}
			} else if f.official(c) != 1 {
				t.Fatal("new confirmation lost")
			}
		})
	}
	for _, rulingFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "new_dispute_first", true: "ruling_first"}[rulingFirst], func(t *testing.T) {
			f := setup(t)
			f.grant("appraiser")
			f.grant("arbiter")
			c := f.create()
			f.prepare(c)
			conf := f.confirm(c)
			x := f.current(c)
			d, e := f.svc.RaiseDispute(f.ctx, f.actors["worker"], f.team, f.project.ID, c, contributions.DisputeInput{VersionCommand: x, ConfirmationID: conf})
			must(t, e)
			ri := contributions.ResolutionInput{Command: f.current(c).Command, Decision: "uphold"}
			di := contributions.DisputeInput{VersionCommand: f.current(c), ConfirmationID: conf}
			resolve := func() error {
				_, e := f.svc.Resolve(f.ctx, f.actors["arbiter"], f.team, f.project.ID, c, d.ID, ri)
				return e
			}
			dispute := func() error {
				_, e := f.svc.RaiseDispute(f.ctx, f.actors["worker"], f.team, f.project.ID, c, di)
				return e
			}
			first, second := dispute, resolve
			if rulingFirst {
				first, second = second, first
			}
			a, b := f.pair(first, second)
			must(t, a)
			status(t, b, 409)
		})
	}
}
func TestServiceConcurrentNoticeIntentIsSingle(t *testing.T) {
	f := setup(t)
	f.grant("appraiser")
	c := f.create()
	f.prepare(c)
	confirmation := f.confirm(c)
	input := func() contributions.NoticeInput {
		return contributions.NoticeInput{VersionCommand: f.current(c), ConfirmationID: confirmation, RecipientID: f.actors["worker"].UserID}
	}
	var first, second uuid.UUID
	a, b := f.pair(func() error {
		out, e := f.svc.PrepareNotice(f.ctx, f.actors["lead"], f.team, f.project.ID, c, input())
		first = out.ID
		return e
	}, func() error {
		out, e := f.svc.PrepareNotice(f.ctx, f.actors["lead"], f.team, f.project.ID, c, input())
		second = out.ID
		return e
	})
	must(t, a)
	must(t, b)
	if first == uuid.Nil || first != second {
		t.Fatal("concurrent identical notice intents returned different notices")
	}
	var n int
	must(t, f.db.Pool.QueryRow(f.ctx, `SELECT count(*) FROM contribution_notifications WHERE team_id=$1 AND project_id=$2 AND task_id=$3 AND case_id=$4 AND confirmation_id=$5 AND recipient_id=$6 AND channel='authenticated_ack'`, f.team, f.project.ID, f.task.ID, c, confirmation, f.actors["worker"].UserID).Scan(&n))
	if n != 1 {
		t.Fatalf("concurrent identical intents created %d notices, want 1", n)
	}
}

func TestServiceConcurrentRevisionAndArchive(t *testing.T) {
	for _, archiveFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "revision_first", true: "archive_first"}[archiveFirst], func(t *testing.T) {
			f := setup(t)
			f.grant("appraiser")
			c := f.create()
			f.prepare(c)
			conf := f.confirm(c)
			n := f.notice(c, conf)
			f.http("POST", f.path(c, "/notices/"+n.String()+"/receipt"), "worker", f.receiptInput(c, conf), 200)
			in := f.current(c)
			revise := func() error {
				_, e := f.svc.VersionAction(f.ctx, f.actors["lead"], f.team, f.project.ID, c, "revisions", in)
				return e
			}
			archive := func() error {
				_, e := f.ps.Archive(f.ctx, f.actors["lead"], f.team, f.project.ID, f.project.Version)
				return e
			}
			first, second := revise, archive
			if archiveFirst {
				first, second = second, first
			}
			a, b := f.pair(first, second)
			must(t, a)
			status(t, b, 409)
		})
	}
	f := setup(t)
	f.grant("appraiser")
	c := f.create()
	f.prepare(c)
	conf := f.confirm(c)
	n := f.notice(c, conf)
	f.http("POST", f.path(c, "/notices/"+n.String()+"/receipt"), "worker", f.receiptInput(c, conf), 200)
	f.http("POST", f.path(c, "/revisions"), "lead", f.current(c), 200)
	f.prepare(c)
	in := f.current(c)
	a, b := f.pair(func() error {
		_, e := f.svc.VersionAction(f.ctx, f.actors["appraiser"], f.team, f.project.ID, c, "confirm", in)
		return e
	}, func() error {
		_, e := f.ps.Archive(f.ctx, f.actors["lead"], f.team, f.project.ID, f.project.Version)
		return e
	})
	must(t, a)
	status(t, b, 409)
}
