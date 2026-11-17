// Package reqctx carries per-request values that are shared by transport and
// infrastructure layers without creating import cycles.
package reqctx

import (
	"context"

	"github.com/google/uuid"
)

type contextKey int

const (
	requestIDKey contextKey = iota
	identityKey
)

// WithRequestID stores the request correlation identifier on the context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// RequestID returns the request correlation identifier, or an empty string
// when the request was not created by the HTTP middleware chain.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// Identity is the authenticated caller resolved from a server-side session.
// Team membership and roles are deliberately absent: they are checked per
// request so that removals and role changes take effect immediately.
type Identity struct {
	UserID        uuid.UUID
	SessionID     uuid.UUID
	Username      string
	DisplayName   string
	SystemRoot    bool
	PlatformAdmin bool
}

// WithIdentity stores the authenticated identity on the context.
func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityKey, identity)
}

// IdentityFrom returns the authenticated identity. The boolean is false for
// unauthenticated requests; handlers behind the auth middleware can rely on it.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityKey).(Identity)
	return identity, ok
}
