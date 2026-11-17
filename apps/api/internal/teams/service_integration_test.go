//go:build integration

package teams

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/audit"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/auth"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/testdb"
)

const teamTestPassword = "team-test-password-1"

type teamsFixture struct {
	svc     *Service
	authSvc *auth.Service
	pool    *pgxpool.Pool
	root    reqctx.Identity
	ctx     context.Context
	t       *testing.T
}

func newTeamsFixture(t *testing.T) *teamsFixture {
	t.Helper()
	dbFixture := testdb.New(t, os.Getenv)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.Up(ctx, dbFixture.SQL, logger); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	auditRepo := audit.NewRepository(dbFixture.Pool)
	authSvc := auth.NewService(auth.NewRepository(dbFixture.Pool), auditRepo, auth.NewLoginLimiter(1000, 15*time.Minute), 12*time.Hour, logger)
	root, err := authSvc.BootstrapRoot(ctx, "root", teamTestPassword)
	if err != nil {
		t.Fatalf("bootstrap root: %v", err)
	}
	svc := NewService(NewRepository(dbFixture.Pool), authSvc, auditRepo, logger)
	return &teamsFixture{
		svc: svc, authSvc: authSvc, pool: dbFixture.Pool,
		root: reqctx.Identity{UserID: root.ID, Username: root.Username, SystemRoot: true},
		ctx:  ctx, t: t,
	}
}

func (f *teamsFixture) account(username string) reqctx.Identity {
	f.t.Helper()
	u, err := f.authSvc.CreateUser(f.ctx, f.root, username, "", teamTestPassword)
	if err != nil {
		f.t.Fatalf("create account %s: %v", username, err)
	}
	return reqctx.Identity{UserID: u.ID, Username: u.Username}
}

func (f *teamsFixture) team(actor reqctx.Identity, name string) Team {
	f.t.Helper()
	summary, err := f.svc.CreateTeam(f.ctx, actor, name, "")
	if err != nil {
		f.t.Fatalf("create team %s: %v", name, err)
	}
	return summary.Team
}

func (f *teamsFixture) join(inviter reqctx.Identity, teamID uuid.UUID, invitee reqctx.Identity, role string) Invitation {
	f.t.Helper()
	inv, err := f.svc.Invite(f.ctx, inviter, teamID, invitee.Username, role, 0)
	if err != nil {
		f.t.Fatalf("invite %s: %v", invitee.Username, err)
	}
	if _, err := f.svc.AcceptInvitation(f.ctx, invitee, inv.ID); err != nil {
		f.t.Fatalf("accept invitation for %s: %v", invitee.Username, err)
	}
	return inv
}

func (f *teamsFixture) role(teamID, userID uuid.UUID) string {
	f.t.Helper()
	m, err := f.svc.repo.Membership(f.ctx, f.pool, teamID, userID)
	if err != nil {
		f.t.Fatalf("membership: %v", err)
	}
	return m.Role
}

func (f *teamsFixture) auditCount(action string, teamID uuid.UUID) int {
	f.t.Helper()
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM audit_events WHERE action = $1 AND team_id = $2`, action, teamID).Scan(&count); err != nil {
		f.t.Fatalf("count audit events: %v", err)
	}
	return count
}

func assertStatus(t *testing.T, err error, want int) {
	t.Helper()
	var apiErr *httpx.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError with status %d, got %v", want, err)
	}
	if apiErr.Status != want {
		t.Fatalf("status = %d (%s), want %d", apiErr.Status, apiErr.Message, want)
	}
}

// twoTeams builds the standard isolation scenario: alice owns team A and is a
// plain member of team B; bob owns team B and is an admin of team A.
func (f *teamsFixture) twoTeams() (alice, bob reqctx.Identity, teamA, teamB Team) {
	f.t.Helper()
	alice = f.account("alice")
	bob = f.account("bob")
	teamA = f.team(alice, "Team Alpha")
	teamB = f.team(bob, "Team Beta")
	f.join(bob, teamB.ID, alice, RoleMember)
	f.join(alice, teamA.ID, bob, RoleAdmin)
	return alice, bob, teamA, teamB
}

// Scenarios 8 and 9: one user belongs to two teams with independent roles.
func TestTwoTeamsIndependentRoles(t *testing.T) {
	f := newTeamsFixture(t)
	alice, bob, teamA, teamB := f.twoTeams()

	if got := f.role(teamA.ID, alice.UserID); got != RoleOwner {
		t.Errorf("alice in A = %s, want owner", got)
	}
	if got := f.role(teamB.ID, alice.UserID); got != RoleMember {
		t.Errorf("alice in B = %s, want member", got)
	}
	if got := f.role(teamB.ID, bob.UserID); got != RoleOwner {
		t.Errorf("bob in B = %s, want owner", got)
	}
	if got := f.role(teamA.ID, bob.UserID); got != RoleAdmin {
		t.Errorf("bob in A = %s, want admin", got)
	}

	summaries, err := f.svc.ListMyTeams(f.ctx, alice)
	if err != nil {
		t.Fatalf("list my teams: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("alice teams = %d, want 2", len(summaries))
	}
	roles := map[uuid.UUID]string{}
	for _, s := range summaries {
		roles[s.ID] = s.MyRole
	}
	if roles[teamA.ID] != RoleOwner || roles[teamB.ID] != RoleMember {
		t.Errorf("unexpected per-team roles: %v", roles)
	}

	permA, err := f.svc.Permissions(f.ctx, alice, teamA.ID)
	if err != nil {
		t.Fatalf("permissions A: %v", err)
	}
	permB, err := f.svc.Permissions(f.ctx, alice, teamB.ID)
	if err != nil {
		t.Fatalf("permissions B: %v", err)
	}
	if !permA.CanInvite || !permA.CanChangeRoles || !permA.CanRemoveMembers {
		t.Errorf("owner capabilities missing in A: %+v", permA)
	}
	if permB.CanInvite || permB.CanChangeRoles || permB.CanRemoveMembers {
		t.Errorf("member must not manage team B: %+v", permB)
	}
	if !permB.CanLeave {
		t.Error("member must be able to leave team B")
	}
}

// Scenario 10: authority in team A never transfers to team B.
func TestRoleInOneTeamGrantsNoAuthorityInAnother(t *testing.T) {
	f := newTeamsFixture(t)
	alice, bob, _, teamB := f.twoTeams()
	carol := f.account("carol")

	_, err := f.svc.Invite(f.ctx, alice, teamB.ID, carol.Username, RoleMember, 0)
	assertStatus(t, err, http.StatusForbidden)

	err = f.svc.ChangeMemberRole(f.ctx, alice, teamB.ID, bob.UserID, RoleMember)
	assertStatus(t, err, http.StatusForbidden)

	err = f.svc.RemoveMember(f.ctx, alice, teamB.ID, bob.UserID)
	assertStatus(t, err, http.StatusForbidden)

	_, err = f.svc.ListTeamInvitations(f.ctx, alice, teamB.ID)
	assertStatus(t, err, http.StatusForbidden)

	// The refusals did not change team B.
	if got := f.role(teamB.ID, bob.UserID); got != RoleOwner {
		t.Errorf("bob in B changed to %s", got)
	}
	if f.auditCount("authz.denied", teamB.ID) < 4 {
		t.Error("expected authorization denials audited for team B")
	}
}

// Scenarios 11 and 13: non-members (including ROOT) cannot see team data.
func TestNonMemberAndRootCannotReadTeam(t *testing.T) {
	f := newTeamsFixture(t)
	alice, _, teamA, _ := f.twoTeams()
	carol := f.account("carol")

	for _, actor := range []reqctx.Identity{carol, f.root} {
		_, err := f.svc.GetTeam(f.ctx, actor, teamA.ID)
		assertStatus(t, err, http.StatusNotFound)
		_, err = f.svc.ListMembers(f.ctx, actor, teamA.ID)
		assertStatus(t, err, http.StatusNotFound)
		_, err = f.svc.Permissions(f.ctx, actor, teamA.ID)
		assertStatus(t, err, http.StatusNotFound)
		_, err = f.svc.ListTeamInvitations(f.ctx, actor, teamA.ID)
		assertStatus(t, err, http.StatusNotFound)
		_, err = f.svc.Invite(f.ctx, actor, teamA.ID, carol.Username, RoleMember, 0)
		assertStatus(t, err, http.StatusNotFound)
		err = f.svc.ChangeMemberRole(f.ctx, actor, teamA.ID, alice.UserID, RoleMember)
		assertStatus(t, err, http.StatusNotFound)
		err = f.svc.RemoveMember(f.ctx, actor, teamA.ID, alice.UserID)
		assertStatus(t, err, http.StatusNotFound)
	}

	// ROOT keeps platform visibility, but that is not team membership.
	teams, err := f.svc.ListAllTeams(f.ctx, f.root)
	if err != nil || len(teams) != 2 {
		t.Fatalf("root list all teams: %v (n=%d)", err, len(teams))
	}
	if _, err := f.svc.ListAllTeams(f.ctx, alice); err == nil {
		t.Fatal("team owner must not list all teams")
	} else {
		assertStatus(t, err, http.StatusForbidden)
	}
}

// Scenario 12: a foreign team_id in the request path cannot reach another
// team's invitation, and accepting writes membership only to the invited team.
func TestCrossTeamInvitationReferenceIsRejected(t *testing.T) {
	f := newTeamsFixture(t)
	alice, bob, teamA, teamB := f.twoTeams()
	carol := f.account("carol")

	inv, err := f.svc.Invite(f.ctx, alice, teamA.ID, carol.Username, RoleMember, 0)
	if err != nil {
		t.Fatalf("invite carol into A: %v", err)
	}

	// bob administers team B but presents team A's invitation under B's path.
	err = f.svc.CancelInvitation(f.ctx, bob, teamB.ID, inv.ID)
	assertStatus(t, err, http.StatusNotFound)
	if inv.Status != InvitationPending {
		t.Fatal("the foreign cancel must not change the invitation")
	}
	var status string
	if err := f.pool.QueryRow(f.ctx, `SELECT status FROM team_invitations WHERE id = $1`, inv.ID).Scan(&status); err != nil || status != InvitationPending {
		t.Fatalf("invitation status = %q (err=%v), want pending", status, err)
	}

	accepted, err := f.svc.AcceptInvitation(f.ctx, carol, inv.ID)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if accepted.TeamID != teamA.ID {
		t.Fatalf("accepted team = %s, want %s", accepted.TeamID, teamA.ID)
	}
	if _, err := f.svc.repo.Membership(f.ctx, f.pool, teamB.ID, carol.UserID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("carol must not appear in team B: %v", err)
	}
	if got := f.role(teamA.ID, carol.UserID); got != RoleMember {
		t.Errorf("carol in A = %s, want member", got)
	}
}

// Scenario 14: removal revokes team access immediately on the same identity,
// while memberships in other teams stay intact.
func TestRemovalTakesEffectImmediately(t *testing.T) {
	f := newTeamsFixture(t)
	alice, bob, teamA, teamB := f.twoTeams()

	if err := f.svc.RemoveMember(f.ctx, bob, teamB.ID, alice.UserID); err != nil {
		t.Fatalf("remove alice from B: %v", err)
	}
	_, err := f.svc.GetTeam(f.ctx, alice, teamB.ID)
	assertStatus(t, err, http.StatusNotFound)
	_, err = f.svc.ListMembers(f.ctx, alice, teamB.ID)
	assertStatus(t, err, http.StatusNotFound)

	summary, err := f.svc.GetTeam(f.ctx, alice, teamA.ID)
	if err != nil || summary.MyRole != RoleOwner {
		t.Fatalf("alice must keep team A: %v %+v", err, summary)
	}
	if f.auditCount("team.member_removed", teamB.ID) != 1 {
		t.Error("removal must be audited exactly once")
	}
}

// Scenario 15: team ownership never implies platform authority.
func TestTeamOwnerIsNotPlatformAdmin(t *testing.T) {
	f := newTeamsFixture(t)
	alice, bob, _, _ := f.twoTeams()

	if alice.SystemRoot || alice.PlatformAdmin {
		t.Fatal("team owner identity must not carry platform flags")
	}
	_, err := f.authSvc.CreateUser(f.ctx, alice, "mallory", "", teamTestPassword)
	assertStatus(t, err, http.StatusForbidden)
	err = f.authSvc.GrantPlatformRole(f.ctx, alice, alice.UserID)
	assertStatus(t, err, http.StatusForbidden)
	err = f.authSvc.GrantPlatformRole(f.ctx, alice, bob.UserID)
	assertStatus(t, err, http.StatusForbidden)

	var platformRoles int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM platform_roles`).Scan(&platformRoles); err != nil {
		t.Fatal(err)
	}
	if platformRoles != 0 {
		t.Fatalf("platform_roles rows = %d, want 0", platformRoles)
	}
}

// Scenario 16: the last active owner cannot be removed or demoted, neither
// through the service nor through direct SQL.
func TestLastOwnerProtection(t *testing.T) {
	f := newTeamsFixture(t)
	alice := f.account("alice")
	bob := f.account("bob")
	team := f.team(alice, "Solo Owner Team")

	err := f.svc.RemoveMember(f.ctx, alice, team.ID, alice.UserID)
	assertStatus(t, err, http.StatusConflict)
	err = f.svc.ChangeMemberRole(f.ctx, alice, team.ID, alice.UserID, RoleMember)
	assertStatus(t, err, http.StatusConflict)

	f.join(alice, team.ID, bob, RoleAdmin)
	err = f.svc.RemoveMember(f.ctx, bob, team.ID, alice.UserID)
	assertStatus(t, err, http.StatusForbidden)
	err = f.svc.ChangeMemberRole(f.ctx, bob, team.ID, alice.UserID, RoleMember)
	assertStatus(t, err, http.StatusForbidden)
	err = f.svc.ChangeMemberRole(f.ctx, bob, team.ID, bob.UserID, RoleOwner)
	assertStatus(t, err, http.StatusForbidden)

	// Direct SQL must fail too: the guard lives in the database.
	if _, err := f.pool.Exec(f.ctx, `DELETE FROM team_members WHERE team_id = $1 AND user_id = $2`, team.ID, alice.UserID); err == nil || !strings.Contains(err.Error(), "active owner") {
		t.Fatalf("direct delete of last owner must fail, got %v", err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE team_members SET role = 'member' WHERE team_id = $1 AND user_id = $2`, team.ID, alice.UserID); err == nil || !strings.Contains(err.Error(), "active owner") {
		t.Fatalf("direct demotion of last owner must fail, got %v", err)
	}

	// With a second owner, handing over and leaving becomes possible.
	if err := f.svc.ChangeMemberRole(f.ctx, alice, team.ID, bob.UserID, RoleOwner); err != nil {
		t.Fatalf("grant second owner: %v", err)
	}
	if err := f.svc.RemoveMember(f.ctx, alice, team.ID, alice.UserID); err != nil {
		t.Fatalf("first owner leaves after handover: %v", err)
	}
	err = f.svc.RemoveMember(f.ctx, bob, team.ID, bob.UserID)
	assertStatus(t, err, http.StatusConflict)
}

// Scenario 17: concurrent mutations preserve every invariant.
func TestConcurrentMutationsPreserveInvariants(t *testing.T) {
	f := newTeamsFixture(t)
	alice := f.account("alice")
	bob := f.account("bob")
	carol := f.account("carol")
	dave := f.account("dave")
	team := f.team(alice, "Concurrency Team")
	f.join(alice, team.ID, bob, RoleAdmin)
	if err := f.svc.ChangeMemberRole(f.ctx, alice, team.ID, bob.UserID, RoleOwner); err != nil {
		t.Fatalf("promote bob to owner: %v", err)
	}
	f.join(alice, team.ID, carol, RoleMember)

	runPair := func(a, b func() error) []error {
		errs := make([]error, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); errs[0] = a() }()
		go func() { defer wg.Done(); errs[1] = b() }()
		wg.Wait()
		return errs
	}

	// Both owners demote themselves at once: exactly one may succeed.
	errs := runPair(
		func() error { return f.svc.ChangeMemberRole(f.ctx, alice, team.ID, alice.UserID, RoleMember) },
		func() error { return f.svc.ChangeMemberRole(f.ctx, bob, team.ID, bob.UserID, RoleMember) },
	)
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else {
			assertStatus(t, err, http.StatusConflict)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent self-demotion successes = %d, want 1 (%v)", successes, errs)
	}
	var owners int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM team_members WHERE team_id = $1 AND status = 'active' AND role = 'owner'`, team.ID).Scan(&owners); err != nil {
		t.Fatal(err)
	}
	if owners != 1 {
		t.Fatalf("active owners after race = %d, want 1", owners)
	}

	// Restore two owners for the next races, acting as the surviving owner.
	var survivingID uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `SELECT user_id FROM team_members WHERE team_id = $1 AND status = 'active' AND role = 'owner'`, team.ID).Scan(&survivingID); err != nil {
		t.Fatal(err)
	}
	surviving, demoted := alice, bob
	if survivingID == bob.UserID {
		surviving, demoted = bob, alice
	}
	if err := f.svc.ChangeMemberRole(f.ctx, surviving, team.ID, demoted.UserID, RoleOwner); err != nil {
		t.Fatalf("restore second owner: %v", err)
	}

	// Concurrent duplicate invitations: exactly one pending row.
	errs = runPair(
		func() error {
			_, err := f.svc.Invite(f.ctx, alice, team.ID, dave.Username, RoleMember, 0)
			return err
		},
		func() error {
			_, err := f.svc.Invite(f.ctx, bob, team.ID, dave.Username, RoleMember, 0)
			return err
		},
	)
	successes = 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else {
			assertStatus(t, err, http.StatusConflict)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent invite successes = %d, want 1 (%v)", successes, errs)
	}
	var pending int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM team_invitations WHERE team_id = $1 AND invited_user_id = $2 AND status = 'pending'`, team.ID, dave.UserID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 1 {
		t.Fatalf("pending invitations = %d, want 1", pending)
	}

	// Concurrent accepts of the same invitation: one membership, one accepted.
	var invID uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `SELECT id FROM team_invitations WHERE team_id = $1 AND invited_user_id = $2 AND status = 'pending'`, team.ID, dave.UserID).Scan(&invID); err != nil {
		t.Fatal(err)
	}
	errs = runPair(
		func() error { _, err := f.svc.AcceptInvitation(f.ctx, dave, invID); return err },
		func() error { _, err := f.svc.AcceptInvitation(f.ctx, dave, invID); return err },
	)
	successes = 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else {
			assertStatus(t, err, http.StatusConflict)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent accept successes = %d, want 1 (%v)", successes, errs)
	}
	var memberships int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM team_members WHERE team_id = $1 AND user_id = $2`, team.ID, dave.UserID).Scan(&memberships); err != nil {
		t.Fatal(err)
	}
	if memberships != 1 {
		t.Fatalf("membership rows for dave = %d, want 1", memberships)
	}

	// Concurrent removals of the same member: one wins, the other sees 404.
	errs = runPair(
		func() error { return f.svc.RemoveMember(f.ctx, alice, team.ID, carol.UserID) },
		func() error { return f.svc.RemoveMember(f.ctx, bob, team.ID, carol.UserID) },
	)
	successes = 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else {
			assertStatus(t, err, http.StatusNotFound)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent removal successes = %d, want 1 (%v)", successes, errs)
	}
	if _, err := f.svc.repo.Membership(f.ctx, f.pool, team.ID, carol.UserID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("carol must be removed: %v", err)
	}
}

// Scenario 18: every permission change is audited in the same transaction,
// and refused mutations leave no audit trail.
func TestPermissionChangesAreAudited(t *testing.T) {
	f := newTeamsFixture(t)
	alice := f.account("alice")
	bob := f.account("bob")
	carol := f.account("carol")
	team := f.team(alice, "Audit Team")

	f.join(alice, team.ID, bob, RoleAdmin)
	if err := f.svc.ChangeMemberRole(f.ctx, alice, team.ID, bob.UserID, RoleMember); err != nil {
		t.Fatalf("change role: %v", err)
	}
	inv2, err := f.svc.Invite(f.ctx, alice, team.ID, carol.Username, RoleMember, 0)
	if err != nil {
		t.Fatalf("invite carol: %v", err)
	}
	if err := f.svc.CancelInvitation(f.ctx, alice, team.ID, inv2.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if err := f.svc.RemoveMember(f.ctx, alice, team.ID, bob.UserID); err != nil {
		t.Fatalf("remove bob: %v", err)
	}

	for action, want := range map[string]int{
		"team.created":              1,
		"team.invitation_created":   2,
		"team.invitation_accepted":  1,
		"team.member_added":         1,
		"team.member_role_changed":  1,
		"team.invitation_cancelled": 1,
		"team.member_removed":       1,
	} {
		if got := f.auditCount(action, team.ID); got != want {
			t.Errorf("audit %s = %d, want %d", action, got, want)
		}
	}

	// A refused last-owner demotion must roll back its audit write too.
	err = f.svc.ChangeMemberRole(f.ctx, alice, team.ID, alice.UserID, RoleMember)
	assertStatus(t, err, http.StatusConflict)
	if got := f.auditCount("team.member_role_changed", team.ID); got != 1 {
		t.Fatalf("refused mutation left audit rows: %d", got)
	}

	// Audit rows carry actor and team context.
	var actorID uuid.UUID
	var details []byte
	if err := f.pool.QueryRow(f.ctx, `SELECT actor_id, details FROM audit_events WHERE action = 'team.member_role_changed' AND team_id = $1`, team.ID).Scan(&actorID, &details); err != nil {
		t.Fatal(err)
	}
	if actorID != alice.UserID {
		t.Error("role change audit must name the acting owner")
	}
	if !strings.Contains(string(details), `"old_role": "admin"`) || !strings.Contains(string(details), `"new_role": "member"`) {
		t.Errorf("role change details missing before/after state: %s", details)
	}
}

func TestInvitationLifecycleRules(t *testing.T) {
	f := newTeamsFixture(t)
	alice := f.account("alice")
	bob := f.account("bob")
	carol := f.account("carol")
	team := f.team(alice, "Invitation Team")

	// Unknown account, invalid role and invalid TTL are rejected.
	_, err := f.svc.Invite(f.ctx, alice, team.ID, "ghost-user", RoleMember, 0)
	assertStatus(t, err, http.StatusNotFound)
	_, err = f.svc.Invite(f.ctx, alice, team.ID, bob.Username, RoleOwner, 0)
	assertStatus(t, err, http.StatusUnprocessableEntity)
	_, err = f.svc.Invite(f.ctx, alice, team.ID, bob.Username, RoleMember, MaxInvitationTTLHours+1)
	assertStatus(t, err, http.StatusUnprocessableEntity)
	_, err = f.svc.Invite(f.ctx, alice, team.ID, bob.Username, RoleMember, -1)
	assertStatus(t, err, http.StatusUnprocessableEntity)

	inv, err := f.svc.Invite(f.ctx, alice, team.ID, bob.Username, RoleMember, 0)
	if err != nil {
		t.Fatalf("invite bob: %v", err)
	}
	if ttl := time.Until(inv.ExpiresAt); ttl < 71*time.Hour || ttl > 73*time.Hour {
		t.Errorf("default TTL = %s, want about 72h", ttl)
	}

	// Duplicate pending invitation and active-member invitation conflict.
	_, err = f.svc.Invite(f.ctx, alice, team.ID, bob.Username, RoleAdmin, 0)
	assertStatus(t, err, http.StatusConflict)

	// Only the invitee may respond; others see 404.
	_, err = f.svc.AcceptInvitation(f.ctx, carol, inv.ID)
	assertStatus(t, err, http.StatusNotFound)
	err = f.svc.DeclineInvitation(f.ctx, carol, inv.ID)
	assertStatus(t, err, http.StatusNotFound)
	_, err = f.svc.AcceptInvitation(f.ctx, alice, inv.ID)
	assertStatus(t, err, http.StatusNotFound)

	if _, err := f.svc.AcceptInvitation(f.ctx, bob, inv.ID); err != nil {
		t.Fatalf("bob accepts: %v", err)
	}
	_, err = f.svc.Invite(f.ctx, alice, team.ID, bob.Username, RoleMember, 0)
	assertStatus(t, err, http.StatusConflict)
	_, err = f.svc.AcceptInvitation(f.ctx, bob, inv.ID)
	assertStatus(t, err, http.StatusConflict)

	// Decline and cancel are terminal for their invitations.
	inv2, err := f.svc.Invite(f.ctx, alice, team.ID, carol.Username, RoleMember, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeclineInvitation(f.ctx, carol, inv2.ID); err != nil {
		t.Fatalf("decline: %v", err)
	}
	_, err = f.svc.AcceptInvitation(f.ctx, carol, inv2.ID)
	assertStatus(t, err, http.StatusConflict)

	// Expired invitations cannot be accepted and disappear from the inbox.
	inv3, err := f.svc.Invite(f.ctx, alice, team.ID, carol.Username, RoleAdmin, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `
		UPDATE team_invitations
		SET created_at = now() - interval '2 hours', expires_at = now() - interval '1 hour', updated_at = now()
		WHERE id = $1`, inv3.ID); err != nil {
		t.Fatalf("expire invitation: %v", err)
	}
	_, err = f.svc.AcceptInvitation(f.ctx, carol, inv3.ID)
	assertStatus(t, err, http.StatusConflict)
	inbox, err := f.svc.ListMyInvitations(f.ctx, carol)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range inbox {
		if item.ID == inv3.ID {
			t.Error("expired invitation must not appear in the inbox")
		}
	}

	// A removed member can be re-invited and reactivated with the new role.
	if err := f.svc.RemoveMember(f.ctx, alice, team.ID, bob.UserID); err != nil {
		t.Fatal(err)
	}
	f.join(alice, team.ID, bob, RoleAdmin)
	if got := f.role(team.ID, bob.UserID); got != RoleAdmin {
		t.Errorf("reactivated role = %s, want admin", got)
	}
	var rows int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM team_members WHERE team_id = $1 AND user_id = $2`, team.ID, bob.UserID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("reactivation duplicated the membership row: %d", rows)
	}
}

func TestCreateTeamValidation(t *testing.T) {
	f := newTeamsFixture(t)
	alice := f.account("alice")

	_, err := f.svc.CreateTeam(f.ctx, alice, "   ", "")
	assertStatus(t, err, http.StatusUnprocessableEntity)
	_, err = f.svc.CreateTeam(f.ctx, alice, strings.Repeat("名", 65), "")
	assertStatus(t, err, http.StatusUnprocessableEntity)
	_, err = f.svc.CreateTeam(f.ctx, alice, "Valid", strings.Repeat("描", 501))
	assertStatus(t, err, http.StatusUnprocessableEntity)

	team, err := f.svc.CreateTeam(f.ctx, alice, strings.Repeat("名", 64), strings.Repeat("描", 500))
	if err != nil {
		t.Fatalf("boundary-length team must be accepted: %v", err)
	}
	if got := f.role(team.ID, alice.UserID); got != RoleOwner {
		t.Errorf("creator role = %s, want owner", got)
	}
	if team.MyRole != RoleOwner {
		t.Errorf("create response my_role = %q, want owner", team.MyRole)
	}
	if team.CreatedAt.IsZero() || team.UpdatedAt.IsZero() {
		t.Errorf("create response must carry real timestamps: %+v", team)
	}
}

func TestPlatformAdminListsAllTeams(t *testing.T) {
	f := newTeamsFixture(t)
	alice := f.account("alice")
	bob := f.account("bob")
	f.team(alice, "Team One")
	f.team(bob, "Team Two")

	if err := f.authSvc.GrantPlatformRole(f.ctx, f.root, bob.UserID); err != nil {
		t.Fatalf("grant platform role: %v", err)
	}
	bob.PlatformAdmin = true
	teams, err := f.svc.ListAllTeams(f.ctx, bob)
	if err != nil || len(teams) != 2 {
		t.Fatalf("platform admin list: %v (n=%d)", err, len(teams))
	}
	// Platform visibility still grants no team business capability.
	var foreignTeam uuid.UUID
	for _, tm := range teams {
		if tm.CreatedBy != bob.UserID {
			foreignTeam = tm.ID
		}
	}
	_, err = f.svc.GetTeam(f.ctx, bob, foreignTeam)
	assertStatus(t, err, http.StatusNotFound)
}

func TestSelfLeaveByMemberAndAdmin(t *testing.T) {
	f := newTeamsFixture(t)
	alice := f.account("alice")
	bob := f.account("bob")
	carol := f.account("carol")
	team := f.team(alice, "Leave Team")
	f.join(alice, team.ID, bob, RoleAdmin)
	f.join(alice, team.ID, carol, RoleMember)

	if err := f.svc.RemoveMember(f.ctx, carol, team.ID, carol.UserID); err != nil {
		t.Fatalf("member self-leave: %v", err)
	}
	if err := f.svc.RemoveMember(f.ctx, bob, team.ID, bob.UserID); err != nil {
		t.Fatalf("admin self-leave: %v", err)
	}
	if _, err := f.svc.repo.Membership(f.ctx, f.pool, team.ID, carol.UserID); !errors.Is(err, ErrNotFound) {
		t.Error("carol must be gone")
	}
	if got := f.role(team.ID, alice.UserID); got != RoleOwner {
		t.Errorf("owner unaffected: %s", got)
	}
	if f.auditCount("team.member_removed", team.ID) != 2 {
		t.Error("both self-leaves must be audited")
	}
}

func TestAdminCannotGrantOwnerOrTouchOwners(t *testing.T) {
	f := newTeamsFixture(t)
	alice := f.account("alice")
	bob := f.account("bob")
	carol := f.account("carol")
	team := f.team(alice, "Admin Limits Team")
	f.join(alice, team.ID, bob, RoleAdmin)
	f.join(alice, team.ID, carol, RoleMember)

	err := f.svc.ChangeMemberRole(f.ctx, bob, team.ID, carol.UserID, RoleOwner)
	assertStatus(t, err, http.StatusForbidden)
	err = f.svc.ChangeMemberRole(f.ctx, bob, team.ID, bob.UserID, RoleOwner)
	assertStatus(t, err, http.StatusForbidden)
	err = f.svc.ChangeMemberRole(f.ctx, bob, team.ID, alice.UserID, RoleAdmin)
	assertStatus(t, err, http.StatusForbidden)
	err = f.svc.RemoveMember(f.ctx, bob, team.ID, alice.UserID)
	assertStatus(t, err, http.StatusForbidden)

	// Admin may still manage plain members.
	if err := f.svc.ChangeMemberRole(f.ctx, bob, team.ID, carol.UserID, RoleAdmin); err != nil {
		t.Fatalf("admin promotes member: %v", err)
	}
	if err := f.svc.RemoveMember(f.ctx, bob, team.ID, carol.UserID); err != nil {
		t.Fatalf("admin removes member: %v", err)
	}
}

func TestMemberListVisibleToAllActiveMembers(t *testing.T) {
	f := newTeamsFixture(t)
	alice := f.account("alice")
	bob := f.account("bob")
	team := f.team(alice, "Visibility Team")
	f.join(alice, team.ID, bob, RoleMember)

	members, err := f.svc.ListMembers(f.ctx, bob, team.ID)
	if err != nil {
		t.Fatalf("member lists members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("members = %d, want 2", len(members))
	}
	for _, m := range members {
		if m.Username == "" || m.Role == "" {
			t.Errorf("member view incomplete: %+v", m)
		}
	}
}

func TestRemovedMemberCannotActEvenAsFormerOwner(t *testing.T) {
	f := newTeamsFixture(t)
	alice := f.account("alice")
	bob := f.account("bob")
	carol := f.account("carol")
	team := f.team(alice, "Handover Team")
	f.join(alice, team.ID, bob, RoleAdmin)
	if err := f.svc.ChangeMemberRole(f.ctx, alice, team.ID, bob.UserID, RoleOwner); err != nil {
		t.Fatalf("promote bob to owner: %v", err)
	}

	if err := f.svc.RemoveMember(f.ctx, bob, team.ID, alice.UserID); err != nil {
		t.Fatalf("second owner removes first: %v", err)
	}
	err := f.svc.ChangeMemberRole(f.ctx, alice, team.ID, bob.UserID, RoleMember)
	assertStatus(t, err, http.StatusNotFound)
	err = f.svc.RemoveMember(f.ctx, alice, team.ID, bob.UserID)
	assertStatus(t, err, http.StatusNotFound)
	_, err = f.svc.Invite(f.ctx, alice, team.ID, carol.Username, RoleMember, 0)
	assertStatus(t, err, http.StatusNotFound)
}

func TestAuditDenialDoesNotBlockRefusal(t *testing.T) {
	f := newTeamsFixture(t)
	alice := f.account("alice")
	carol := f.account("carol")
	team := f.team(alice, "Denial Audit Team")

	_, err := f.svc.GetTeam(f.ctx, carol, team.ID)
	assertStatus(t, err, http.StatusNotFound)
	if f.auditCount("authz.denied", team.ID) < 1 {
		t.Error("denial must be audited")
	}
	var details []byte
	if err := f.pool.QueryRow(f.ctx, `SELECT details FROM audit_events WHERE action = 'authz.denied' AND team_id = $1 LIMIT 1`, team.ID).Scan(&details); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(details), "not_a_member") {
		t.Errorf("denial reason missing: %s", details)
	}
}

func TestTeamNamesAreScopedPerTeamNotGlobal(t *testing.T) {
	f := newTeamsFixture(t)
	alice := f.account("alice")
	bob := f.account("bob")
	if _, err := f.svc.CreateTeam(f.ctx, alice, "Same Name", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateTeam(f.ctx, bob, "Same Name", ""); err != nil {
		t.Fatalf("team names must not be globally unique: %v", err)
	}
}

func TestInvitationDefaultAndCustomTTL(t *testing.T) {
	f := newTeamsFixture(t)
	alice := f.account("alice")
	bob := f.account("bob")
	team := f.team(alice, "TTL Team")

	inv, err := f.svc.Invite(f.ctx, alice, team.ID, bob.Username, RoleMember, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ttl := time.Until(inv.ExpiresAt); ttl < 55*time.Minute || ttl > 65*time.Minute {
		t.Errorf("custom TTL = %s, want about 1h", ttl)
	}
	if err := f.svc.CancelInvitation(f.ctx, alice, team.ID, inv.ID); err != nil {
		t.Fatal(err)
	}
	err = f.svc.CancelInvitation(f.ctx, alice, team.ID, inv.ID)
	assertStatus(t, err, http.StatusConflict)
	_, err = f.svc.AcceptInvitation(f.ctx, bob, inv.ID)
	assertStatus(t, err, http.StatusConflict)
}
