// Package httpapi wires HTTP routes to application behaviour.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/auth"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/config"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/observability"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/teams"
)

// SystemStore is the slice of platform state the HTTP layer needs. Keeping it
// narrow makes handler tests independent from PostgreSQL.
type SystemStore interface {
	Ping(ctx context.Context) error
	InstallationID(ctx context.Context) (string, error)
	SchemaVersion(ctx context.Context) (int64, error)
}

type RouteModule interface{ AuthenticatedRoutes(chi.Router) }

// Server holds the dependencies shared by all handlers.
type Server struct {
	cfg              config.Config
	logger           *slog.Logger
	system           SystemStore
	authSvc          *auth.Service
	authHandler      *auth.Handler
	teamsHandler     *teams.Handler
	businessHandlers []RouteModule
}

// New builds an HTTP server binding. The auth dependencies may be nil in
// tests that only exercise diagnostics; auth and team routes are then not
// mounted. The teams handler requires the auth middleware to be active.
func New(cfg config.Config, logger *slog.Logger, system SystemStore, authSvc *auth.Service, authHandler *auth.Handler, teamsHandler *teams.Handler, modules ...RouteModule) *Server {
	server := &Server{cfg: cfg, logger: logger, system: system, authSvc: authSvc, authHandler: authHandler, teamsHandler: teamsHandler}
	server.businessHandlers = modules
	return server
}

// Handler returns the fully configured router.
func (s *Server) Handler() http.Handler {
	router := chi.NewRouter()
	router.Use(observability.RequestID)
	router.Use(observability.AccessLog(s.logger))
	router.Use(observability.Recoverer(s.logger))
	router.Use(observability.SecurityHeaders(s.cfg.Environment == config.EnvironmentProduction))

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, r, s.logger, httpx.NotFound("the requested resource does not exist"))
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, r, s.logger, &httpx.APIError{
			Status:  http.StatusMethodNotAllowed,
			Code:    "method_not_allowed",
			Message: "the requested method is not allowed for this resource",
		})
	})

	router.Get("/healthz", s.handleLiveness)
	router.Get("/readyz", s.handleReadiness)

	router.Route("/api/v1", func(r chi.Router) {
		// CSRF guards every mutating /api/v1 route, including the unauthenticated
		// login POST: a cross-site form cannot set this custom header.
		r.Use(auth.CSRF)
		r.Get("/meta", s.handleMeta)
		if s.authHandler != nil {
			s.authHandler.PublicRoutes(r)
			r.Group(func(r chi.Router) {
				r.Use(auth.Middleware(s.authSvc))
				s.authHandler.AuthenticatedRoutes(r)
				if s.teamsHandler != nil {
					s.teamsHandler.AuthenticatedRoutes(r)
				}
				for _, module := range s.businessHandlers {
					module.AuthenticatedRoutes(r)
				}
			})
		}
	})

	return router
}
