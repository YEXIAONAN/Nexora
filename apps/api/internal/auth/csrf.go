package auth

import (
	"net/http"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
)

// CSRFHeader must be present on mutating API requests. A custom header cannot
// be set by cross-origin forms or simple requests, and no CORS policy allows
// foreign origins, so this plus SameSite=Lax cookies blocks CSRF (ADR 0006).
const CSRFHeader = "X-Nexora-CSRF"

// CSRF protects mutating methods. Safe methods pass through untouched.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if r.Header.Get(CSRFHeader) != "1" {
				httpx.WriteError(w, r, nil, httpx.Forbidden("mutating requests require the "+CSRFHeader+" header"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
