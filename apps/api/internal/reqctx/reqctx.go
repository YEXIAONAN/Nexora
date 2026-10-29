// Package reqctx carries per-request values that are shared by transport and
// infrastructure layers without creating import cycles.
package reqctx

import "context"

type contextKey int

const requestIDKey contextKey = iota

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
