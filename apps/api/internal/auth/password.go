package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters follow RFC 9106 option 2 (t=3, m=64MiB, p=4). The
// benchmark test records the actual per-hash cost on the running hardware;
// raise the parameters only with evidence, not by guessing.
const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32
	argonSaltLen        = 16

	minPasswordBytes = 10
	maxPasswordBytes = 128
	maxDisplayName   = 64
)

// HashPassword returns the PHC string format
// $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword recomputes the hash with the stored parameters and compares
// in constant time. Malformed encodings fail closed.
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash equalizes timing for unknown usernames: verification runs against
// a real hash even when no account exists.
var dummyHash = mustHash("nexora-dummy-password-for-timing-equalization")

func mustHash(password string) string {
	encoded, err := HashPassword(password)
	if err != nil {
		panic("auth: cannot initialize dummy password hash: " + err.Error())
	}
	return encoded
}

// VerifyAgainstDummy burns the same Argon2id cost as a real verification.
func VerifyAgainstDummy(password string) {
	VerifyPassword(dummyHash, password)
}

// PasswordFieldError validates the password policy and returns a field error
// message, or an empty string when the password is acceptable.
func PasswordFieldError(username, password string) string {
	switch {
	case len(password) < minPasswordBytes:
		return fmt.Sprintf("must be at least %d bytes", minPasswordBytes)
	case len(password) > maxPasswordBytes:
		return fmt.Sprintf("must be at most %d bytes", maxPasswordBytes)
	case strings.TrimSpace(password) == "":
		return "must not be blank"
	case strings.EqualFold(password, username):
		return "must not equal the username"
	default:
		return ""
	}
}

// UsernameFieldError validates the username format after lowercasing.
func UsernameFieldError(username string) string {
	if len(username) < 3 || len(username) > 32 {
		return "must be 3-32 characters"
	}
	for _, r := range username {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return "may only contain lowercase letters, digits, '_' and '-'"
	}
	return ""
}

// DisplayNameFieldError validates the display name length.
func DisplayNameFieldError(displayName string) string {
	if utf8.RuneCountInString(displayName) > maxDisplayName {
		return fmt.Sprintf("must be at most %d characters", maxDisplayName)
	}
	return ""
}
