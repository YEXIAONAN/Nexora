package auth

import (
	"errors"
	"net/http"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

// Middleware resolves the session cookie into a request identity. Expired,
// revoked and unknown tokens all produce the same 401 response.
func Middleware(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(SessionCookieName)
			if err != nil || cookie.Value == "" {
				httpx.WriteError(w, r, nil, httpx.Unauthorized("authentication required"))
				return
			}
			identity, err := svc.Authenticate(r.Context(), cookie.Value)
			if errors.Is(err, ErrNotFound) {
				httpx.WriteError(w, r, nil, httpx.Unauthorized("the session is invalid or has expired"))
				return
			}
			if err != nil {
				httpx.WriteError(w, r, nil, httpx.Internal(err))
				return
			}
			next.ServeHTTP(w, r.WithContext(reqctx.WithIdentity(r.Context(), identity)))
		})
	}
}
