//go:build integration

package auth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/audit"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/testdb"
)

const testRootPassword = "root-bootstrap-password-1"

type authFixture struct {
	svc  *Service
	repo *Repository
	ctx  context.Context
}

func newAuthFixture(t *testing.T) *authFixture {
	t.Helper()
	fixture := testdb.New(t, os.Getenv)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	if err := database.Up(ctx, fixture.SQL, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	repo := NewRepository(fixture.Pool)
	svc := NewService(repo, audit.NewRepository(fixture.Pool), NewLoginLimiter(10, 15*time.Minute), time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return &authFixture{svc: svc, repo: repo, ctx: ctx}
}

func (f *authFixture) bootstrapRoot(t *testing.T, username string) User {
	t.Helper()
	user, err := f.svc.BootstrapRoot(f.ctx, username, testRootPassword)
	if err != nil {
		t.Fatalf("bootstrap root: %v", err)
	}
	return user
}

func (f *authFixture) createUser(t *testing.T, actor reqctx.Identity, username string) User {
	t.Helper()
	user, err := f.svc.CreateUser(f.ctx, actor, username, "", "account-password-123")
	if err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	return user
}

func (f *authFixture) login(t *testing.T, username, password string) LoginResult {
	t.Helper()
	result, err := f.svc.Login(f.ctx, username, password, "integration-test", "127.0.0.1")
	if err != nil {
		t.Fatalf("login %s: %v", username, err)
	}
	return result
}

func (f *authFixture) auditCount(t *testing.T, action string) int {
	t.Helper()
	var count int
	if err := f.repo.pool.QueryRow(f.ctx, `SELECT count(*) FROM audit_events WHERE action = $1`, action).Scan(&count); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	return count
}

func rootIdentity(user User) reqctx.Identity {
	return reqctx.Identity{UserID: user.ID, Username: user.Username, SystemRoot: true}
}

func userIdentity(user User) reqctx.Identity {
	return reqctx.Identity{UserID: user.ID, Username: user.Username}
}

// Security scenario 1: no default ROOT password exists; bootstrap requires an
// explicit strong password and weak defaults are rejected.
func TestBootstrapRequiresExplicitStrongPassword(t *testing.T) {
	f := newAuthFixture(t)
	for _, password := range []string{"", "123456", "waiting", "short"} {
		if _, err := f.svc.BootstrapRoot(f.ctx, "waiting", password); err == nil {
			t.Fatalf("weak password %q accepted", password)
		}
	}
	var count int
	if err := f.repo.pool.QueryRow(f.ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rejected bootstraps created %d users", count)
	}
}

func TestBootstrapCreatesRootAndAudit(t *testing.T) {
	f := newAuthFixture(t)
	root := f.bootstrapRoot(t, "waiting")
	if !root.SystemRoot {
		t.Fatal("bootstrapped account must carry system_root")
	}
	initialized, err := f.svc.RootInitialized(f.ctx)
	if err != nil || !initialized {
		t.Fatalf("RootInitialized = %v, %v", initialized, err)
	}
	if got := f.auditCount(t, "root.bootstrap"); got != 1 {
		t.Fatalf("root.bootstrap audit count = %d, want 1", got)
	}
}

// Security scenario 3: repeated bootstrap is refused safely.
func TestBootstrapRefusesRepeat(t *testing.T) {
	f := newAuthFixture(t)
	f.bootstrapRoot(t, "waiting")
	if _, err := f.svc.BootstrapRoot(f.ctx, "another-root", "another-password-123"); !errors.Is(err, ErrRootAlreadyInitialized) {
		t.Fatalf("second bootstrap error = %v, want ErrRootAlreadyInitialized", err)
	}
	var count int
	if err := f.repo.pool.QueryRow(f.ctx, `SELECT count(*) FROM users WHERE system_root`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("system_root count = %d, want 1", count)
	}
}

// Security scenario 4: concurrent bootstrap cannot create two ROOTs.
func TestBootstrapConcurrentCreatesExactlyOneRoot(t *testing.T) {
	f := newAuthFixture(t)
	const workers = 8
	var wg sync.WaitGroup
	results := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.svc.BootstrapRoot(f.ctx, "waiting", testRootPassword)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrRootAlreadyInitialized), errors.Is(err, ErrUsernameTaken):
			// Both unique constraints are safe refusals: whichever fires
			// first, no second ROOT may be created.
		default:
			t.Fatalf("unexpected bootstrap error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful bootstraps = %d, want exactly 1", successes)
	}
	var count int
	if err := f.repo.pool.QueryRow(f.ctx, `SELECT count(*) FROM users WHERE system_root`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("system_root count = %d, want 1", count)
	}
}

// The waiting username is only a default label: ROOT authority comes from the
// immutable flag, and an ordinary account named waiting gains nothing.
func TestWaitingUsernameDoesNotGrantRoot(t *testing.T) {
	f := newAuthFixture(t)
	root := f.bootstrapRoot(t, "platform-root")
	waiting := f.createUser(t, rootIdentity(root), "waiting")
	if waiting.SystemRoot {
		t.Fatal("ordinary account named waiting must not be ROOT")
	}
	result := f.login(t, "waiting", "account-password-123")
	identity, err := f.svc.Authenticate(f.ctx, result.Token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if identity.SystemRoot || identity.PlatformAdmin {
		t.Fatal("waiting must hold no platform authority")
	}
}

func TestLoginStoresOnlyTokenDigest(t *testing.T) {
	f := newAuthFixture(t)
	root := f.bootstrapRoot(t, "waiting")
	result := f.login(t, "waiting", testRootPassword)
	if result.Token == "" {
		t.Fatal("empty session token")
	}
	var stored string
	if err := f.repo.pool.QueryRow(f.ctx, `SELECT token_hash FROM sessions WHERE id = $1`, result.SessionID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == result.Token {
		t.Fatal("raw session token must never be stored")
	}
	if stored != hashToken(result.Token) {
		t.Fatal("stored digest must be SHA-256 of the token")
	}
	identity, err := f.svc.Authenticate(f.ctx, result.Token)
	if err != nil || identity.UserID != root.ID || !identity.SystemRoot {
		t.Fatalf("authenticate = %+v, %v", identity, err)
	}
	if got := f.auditCount(t, "auth.login_success"); got != 1 {
		t.Fatalf("login_success audit count = %d", got)
	}
}

// Anti-enumeration: unknown users and wrong passwords return the same error.
func TestLoginHidesAccountExistence(t *testing.T) {
	f := newAuthFixture(t)
	f.bootstrapRoot(t, "waiting")
	_, unknownErr := f.svc.Login(f.ctx, "ghost-user", "whatever-password-1", "t", "127.0.0.1")
	_, wrongErr := f.svc.Login(f.ctx, "waiting", "wrong-password-123", "t", "127.0.0.1")
	if !errors.Is(unknownErr, ErrInvalidCredentials) || !errors.Is(wrongErr, ErrInvalidCredentials) {
		t.Fatalf("errors differ: unknown=%v wrong=%v", unknownErr, wrongErr)
	}
	if got := f.auditCount(t, "auth.login_failed"); got != 2 {
		t.Fatalf("login_failed audit count = %d, want 2", got)
	}
	var details string
	if err := f.repo.pool.QueryRow(f.ctx, `SELECT details::text FROM audit_events WHERE action = 'auth.login_failed' LIMIT 1`).Scan(&details); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(details, "invalid_credentials") {
		t.Fatalf("audit details missing reason: %s", details)
	}
	if strings.Contains(details, "whatever-password") || strings.Contains(details, "wrong-password") {
		t.Fatal("audit must never record passwords")
	}
}

// Security scenario 6: login failure rate limiting.
func TestLoginRateLimit(t *testing.T) {
	f := newAuthFixture(t)
	f.bootstrapRoot(t, "waiting")
	f.svc.limiter = NewLoginLimiter(3, 15*time.Minute)
	for range 3 {
		if _, err := f.svc.Login(f.ctx, "waiting", "wrong-password-123", "t", "127.0.0.1"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("expected invalid credentials, got %v", err)
		}
	}
	_, err := f.svc.Login(f.ctx, "waiting", testRootPassword, "t", "127.0.0.1")
	var apiErr *httpx.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 429 || apiErr.Code != "rate_limited" {
		t.Fatalf("fourth attempt = %v, want 429 rate_limited", err)
	}
	if _, err := f.svc.Login(f.ctx, "other-user", "wrong-password-123", "t", "127.0.0.1"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("different username must not be limited: %v", err)
	}
}

// Security scenario 5: revoked sessions cannot continue.
func TestLogoutRevokesSession(t *testing.T) {
	f := newAuthFixture(t)
	f.bootstrapRoot(t, "waiting")
	result := f.login(t, "waiting", testRootPassword)
	identity, err := f.svc.Authenticate(f.ctx, result.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Logout(f.ctx, identity); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := f.svc.Authenticate(f.ctx, result.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked session still authenticates: %v", err)
	}
	if got := f.auditCount(t, "auth.logout"); got != 1 {
		t.Fatalf("logout audit count = %d", got)
	}
}

func TestExpiredSessionCannotAuthenticate(t *testing.T) {
	f := newAuthFixture(t)
	f.bootstrapRoot(t, "waiting")
	result := f.login(t, "waiting", testRootPassword)
	if _, err := f.repo.pool.Exec(f.ctx, `UPDATE sessions SET expires_at = created_at - interval '1 second' WHERE id = $1`, result.SessionID); err == nil {
		t.Fatal("database must refuse expires_at <= created_at")
	}
	// Shift the whole validity window into the past; the CHECK constraint
	// correctly refuses expires_at <= created_at.
	if _, err := f.repo.pool.Exec(f.ctx, `
		UPDATE sessions SET created_at = now() - interval '2 hours', expires_at = now() - interval '1 hour'
		WHERE id = $1`, result.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Authenticate(f.ctx, result.Token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session still authenticates: %v", err)
	}
}

// Session fixation: every login issues a fresh token.
func TestLoginAlwaysIssuesFreshToken(t *testing.T) {
	f := newAuthFixture(t)
	f.bootstrapRoot(t, "waiting")
	first := f.login(t, "waiting", testRootPassword)
	second := f.login(t, "waiting", testRootPassword)
	if first.Token == second.Token {
		t.Fatal("repeated login reused the session token")
	}
	if _, err := f.svc.Authenticate(f.ctx, first.Token); err != nil {
		t.Fatalf("first session should remain valid until expiry/revocation: %v", err)
	}
}

// Security scenario 2 (identity part): ordinary accounts cannot modify the
// ROOT identity, and platform role management is ROOT-only.
func TestPlatformRoleManagementIsRootOnly(t *testing.T) {
	f := newAuthFixture(t)
	root := f.bootstrapRoot(t, "waiting")
	alice := f.createUser(t, rootIdentity(root), "alice")

	if err := f.svc.GrantPlatformRole(f.ctx, userIdentity(alice), alice.ID); !httpx.IsAPIError(err) {
		t.Fatalf("non-root grant = %v, want forbidden", err)
	}
	if err := f.svc.GrantPlatformRole(f.ctx, rootIdentity(root), root.ID); err == nil {
		t.Fatal("ROOT must not receive platform roles")
	}
	if err := f.svc.GrantPlatformRole(f.ctx, rootIdentity(root), alice.ID); err != nil {
		t.Fatalf("root grant: %v", err)
	}
	if err := f.svc.GrantPlatformRole(f.ctx, rootIdentity(root), alice.ID); err == nil {
		t.Fatal("duplicate grant must conflict")
	}

	result := f.login(t, "alice", "account-password-123")
	identity, err := f.svc.Authenticate(f.ctx, result.Token)
	if err != nil || !identity.PlatformAdmin {
		t.Fatalf("granted role not visible: %+v %v", identity, err)
	}
	if identity.SystemRoot {
		t.Fatal("platform admin must not become ROOT")
	}

	if err := f.svc.RevokePlatformRole(f.ctx, userIdentity(alice), alice.ID); err == nil {
		t.Fatal("platform admin must not revoke platform roles")
	}
	if err := f.svc.RevokePlatformRole(f.ctx, rootIdentity(root), alice.ID); err != nil {
		t.Fatalf("root revoke: %v", err)
	}
	identity, err = f.svc.Authenticate(f.ctx, result.Token)
	if err != nil || identity.PlatformAdmin {
		t.Fatalf("revoked role still visible: %+v %v", identity, err)
	}
	if got := f.auditCount(t, "platform_role.granted"); got != 1 {
		t.Fatalf("granted audit count = %d", got)
	}
	if got := f.auditCount(t, "platform_role.revoked"); got != 1 {
		t.Fatalf("revoked audit count = %d", got)
	}
}

func TestCreateUserAuthorization(t *testing.T) {
	f := newAuthFixture(t)
	root := f.bootstrapRoot(t, "waiting")
	alice := f.createUser(t, rootIdentity(root), "alice")

	if _, err := f.svc.CreateUser(f.ctx, userIdentity(alice), "bob", "", "account-password-123"); err == nil {
		t.Fatal("ordinary member must not create accounts")
	}
	if _, err := f.svc.CreateUser(f.ctx, rootIdentity(root), "alice", "", "account-password-123"); err == nil {
		t.Fatal("duplicate username must conflict")
	}
	if err := f.svc.GrantPlatformRole(f.ctx, rootIdentity(root), alice.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CreateUser(f.ctx, reqctx.Identity{UserID: alice.ID, PlatformAdmin: true}, "bob", "", "account-password-123"); err != nil {
		t.Fatalf("platform admin must create accounts: %v", err)
	}
	if got := f.auditCount(t, "user.created"); got != 2 {
		t.Fatalf("user.created audit count = %d, want 2", got)
	}
}

func TestAdminRevokeUserSessions(t *testing.T) {
	f := newAuthFixture(t)
	root := f.bootstrapRoot(t, "waiting")
	alice := f.createUser(t, rootIdentity(root), "alice")
	first := f.login(t, "alice", "account-password-123")
	second := f.login(t, "alice", "account-password-123")

	if _, err := f.svc.RevokeUserSessions(f.ctx, userIdentity(alice), alice.ID); err == nil {
		t.Fatal("ordinary member must not revoke sessions")
	}
	count, err := f.svc.RevokeUserSessions(f.ctx, rootIdentity(root), alice.ID)
	if err != nil || count != 2 {
		t.Fatalf("revoke count = %d, %v; want 2", count, err)
	}
	for _, token := range []string{first.Token, second.Token} {
		if _, err := f.svc.Authenticate(f.ctx, token); !errors.Is(err, ErrNotFound) {
			t.Fatalf("session survived admin revocation: %v", err)
		}
	}
	if got := f.auditCount(t, "auth.sessions_revoked"); got != 1 {
		t.Fatalf("sessions_revoked audit count = %d", got)
	}
}

func TestAuditEventsAreAppendOnly(t *testing.T) {
	f := newAuthFixture(t)
	f.bootstrapRoot(t, "waiting")
	if _, err := f.repo.pool.Exec(f.ctx, `UPDATE audit_events SET action = 'tampered'`); err == nil {
		t.Fatal("UPDATE on audit_events must be refused by the database")
	}
	if _, err := f.repo.pool.Exec(f.ctx, `DELETE FROM audit_events`); err == nil {
		t.Fatal("DELETE on audit_events must be refused by the database")
	}
	var action string
	if err := f.repo.pool.QueryRow(f.ctx, `SELECT action FROM audit_events LIMIT 1`).Scan(&action); err != nil {
		t.Fatal(err)
	}
	if action != "root.bootstrap" {
		t.Fatalf("audit row mutated: %q", action)
	}
}

// Permission changes and their audit commit or roll back together.
func TestFailedPermissionChangeLeavesNoAudit(t *testing.T) {
	f := newAuthFixture(t)
	root := f.bootstrapRoot(t, "waiting")
	before := f.auditCount(t, "platform_role.granted")
	if err := f.svc.GrantPlatformRole(f.ctx, rootIdentity(root), uuid.New()); err == nil {
		t.Fatal("grant to unknown user must fail")
	}
	if after := f.auditCount(t, "platform_role.granted"); after != before {
		t.Fatalf("failed grant wrote audit: %d -> %d", before, after)
	}
}
