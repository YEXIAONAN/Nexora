package observability

import (
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

const requestIDHeader = "X-Request-Id"

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// RequestID ensures every request has a correlation identifier. A well-formed
// inbound X-Request-Id is preserved unless it reflects a cookie credential;
// unsafe values are replaced with a new UUID.
// The identifier is echoed in the response and in error envelopes.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get(requestIDHeader))
		// Correlation values are echoed and audited. Never promote a request's
		// cookie value into either channel, even when it matches the ID syntax.
		for _, cookie := range r.Cookies() {
			if cookie.Value != "" && id == cookie.Value {
				id = ""
				break
			}
		}
		if !validRequestID.MatchString(id) {
			id = uuid.NewString()
		}
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(reqctx.WithRequestID(r.Context(), id)))
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (r *responseRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// AccessLog emits one structured log line per request with the matched route
// pattern rather than the raw path so that identifiers do not explode cardinality.
func AccessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := &responseRecorder{ResponseWriter: w}
			next.ServeHTTP(recorder, r)

			status := recorder.status
			if status == 0 {
				status = http.StatusOK
			}
			route := r.URL.Path
			if routeCtx := chi.RouteContext(r.Context()); routeCtx != nil && routeCtx.RoutePattern() != "" {
				route = routeCtx.RoutePattern()
			}

			attrs := []any{
				"method", r.Method,
				"route", route,
				"status", status,
				"bytes", recorder.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", reqctx.RequestID(r.Context()),
				"remote_ip", r.RemoteAddr,
			}
			switch {
			case status >= http.StatusInternalServerError:
				logger.Error("http request", attrs...)
			case status >= http.StatusBadRequest:
				logger.Warn("http request", attrs...)
			default:
				logger.Info("http request", attrs...)
			}
		})
	}
}

// Recoverer converts a panic into a logged 500 response instead of killing the
// process or leaking a stack trace to the client.
func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}
				logger.Error("panic recovered",
					"panic", fmt.Sprint(recovered),
					"stack", string(debug.Stack()),
					"method", r.Method,
					"path", r.URL.Path,
					"request_id", reqctx.RequestID(r.Context()),
				)
				httpx.WriteError(w, r, logger, httpx.Internal(fmt.Errorf("panic: %v", recovered)))
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders applies conservative defaults for a JSON API. HSTS is only
// emitted in production so that local HTTP development keeps working.
func SecurityHeaders(production bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := w.Header()
			header.Set("X-Content-Type-Options", "nosniff")
			header.Set("X-Frame-Options", "DENY")
			header.Set("Referrer-Policy", "no-referrer")
			header.Set("Cross-Origin-Resource-Policy", "same-site")
			header.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
			if production {
				header.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}
