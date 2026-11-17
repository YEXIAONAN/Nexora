package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordRoundtrip(t *testing.T) {
	encoded, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("unexpected PHC format: %q", encoded)
	}
	if !VerifyPassword(encoded, "correct horse battery staple") {
		t.Fatal("correct password rejected")
	}
	if VerifyPassword(encoded, "wrong password") {
		t.Fatal("wrong password accepted")
	}
}

func TestHashPasswordUsesFreshSalt(t *testing.T) {
	first, err := HashPassword("same password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	second, err := HashPassword("same password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if first == second {
		t.Fatal("identical passwords must not produce identical hashes")
	}
}

func TestVerifyPasswordFailsClosedOnMalformedEncoding(t *testing.T) {
	for _, encoded := range []string{
		"",
		"not-a-hash",
		"$argon2id$v=19$m=65536,t=3,p=4$badsalt$badhash",
		"$argon2i$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA",
		"$argon2id$v=18$m=65536,t=3,p=4$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=abc,t=3,p=4$c2FsdA$aGFzaA",
	} {
		if VerifyPassword(encoded, "password") {
			t.Errorf("malformed encoding accepted: %q", encoded)
		}
	}
}

func TestPasswordFieldPolicy(t *testing.T) {
	for _, tc := range []struct {
		name     string
		username string
		password string
		wantErr  bool
	}{
		{"valid", "alice", "a-strong-password", false},
		{"too short", "alice", "short", true},
		{"too long", "alice", strings.Repeat("x", 129), true},
		{"max length ok", "alice", strings.Repeat("x", 128), false},
		{"blank", "alice", "           ", true},
		{"equals username", "alice", "alice", true},
		{"equals username case insensitive", "alice", "ALICE", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := PasswordFieldError(tc.username, tc.password) != ""
			if got != tc.wantErr {
				t.Fatalf("PasswordFieldError(%q) error=%v, want %v", tc.password, got, tc.wantErr)
			}
		})
	}
}

func TestUsernameFieldPolicy(t *testing.T) {
	for _, tc := range []struct {
		username string
		wantErr  bool
	}{
		{"waiting", false},
		{"zero_r", false},
		{"team-01", false},
		{"ab", true},
		{strings.Repeat("a", 33), true},
		{"Upper", true},
		{"has space", true},
		{"héllo", true},
	} {
		t.Run(tc.username, func(t *testing.T) {
			got := UsernameFieldError(tc.username) != ""
			if got != tc.wantErr {
				t.Fatalf("UsernameFieldError(%q) error=%v, want %v", tc.username, got, tc.wantErr)
			}
		})
	}
}

func TestDisplayNameFieldPolicy(t *testing.T) {
	if DisplayNameFieldError(strings.Repeat("名", 64)) != "" {
		t.Fatal("64 runes must be accepted")
	}
	if DisplayNameFieldError(strings.Repeat("名", 65)) == "" {
		t.Fatal("65 runes must be rejected")
	}
}

// BenchmarkHashPassword records the real per-hash cost on this hardware so
// parameter choices stay evidence-based (ADR 0006).
func BenchmarkHashPassword(b *testing.B) {
	for b.Loop() {
		if _, err := HashPassword("benchmark-password-value"); err != nil {
			b.Fatal(err)
		}
	}
}
