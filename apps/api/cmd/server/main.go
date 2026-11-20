// Command server runs the Nexora HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/audit"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/auth"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/config"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/contributions"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpapi"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/observability"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/projects"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/submissions"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/system"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/tasks"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/teams"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "nexora-api: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logger := observability.NewLogger(cfg.LogLevel)
	build := version.Get()
	logger.Info("starting nexora-api",
		"version", build.Version,
		"commit", build.Commit,
		"environment", string(cfg.Environment),
		"http_addr", cfg.HTTPAddr,
		"database", cfg.RedactedDatabaseURL(),
	)

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.MigrateOnStart {
		logger.Warn("NEXORA_MIGRATE_ON_START is enabled; applying migrations before serving traffic")
		if err := migrateOnStart(rootCtx, cfg, logger); err != nil {
			return err
		}
	}

	pool, err := database.Open(rootCtx, cfg)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()
	if cfg.Environment == config.EnvironmentProduction {
		if err := database.ValidateApplicationRole(rootCtx, pool); err != nil {
			return err
		}
	}

	var schemaVersion int64
	if e := pool.QueryRow(rootCtx, `SELECT COALESCE(max(version_id),0) FROM goose_db_version WHERE is_applied`).Scan(&schemaVersion); e != nil {
		return fmt.Errorf("read schema version before startup: %w", database.SafeError(e))
	}
	if !database.CompatibleSchemaVersion(schemaVersion) {
		return fmt.Errorf("API requires schema v%d–v%d; current schema v%d; apply reviewed migrations explicitly before startup",
			database.MinimumSupportedSchemaVersion, database.MaximumSupportedSchemaVersion, schemaVersion)
	}
	store, err := submissions.OpenStore(cfg.AttachmentDir)
	if err != nil {
		return fmt.Errorf("open private attachment storage: %w", err)
	}
	defer store.Close()
	auditRepo := audit.NewRepository(pool)
	authSvc := auth.NewService(
		auth.NewRepository(pool),
		auditRepo,
		auth.NewLoginLimiter(cfg.LoginMaxFailures, cfg.LoginWindow),
		cfg.SessionTTL,
		logger,
	)
	authSvc.SetRegistrationMode(auth.RegistrationMode(cfg.RegistrationMode))
	authHandler := auth.NewHandler(authSvc, cfg.CookieSecure(), cfg.SessionTTL, logger)
	teamsSvc := teams.NewService(teams.NewRepository(pool), authSvc, auditRepo, logger)
	authSvc.SetInvitationChecker(teamsSvc)
	teamsHandler := teams.NewHandler(teamsSvc, logger)
	projectsSvc := projects.NewService(projects.NewRepository(pool), teams.NewRepository(pool), auditRepo, logger)
	projectsHandler := projects.NewHandler(projectsSvc, logger)
	tasksSvc := tasks.NewService(tasks.NewRepository(), projectsSvc)
	tasksHandler := tasks.NewHandler(tasksSvc, logger)
	submissionsSvc := submissions.NewService(submissions.NewRepository(pool), projectsSvc, tasksSvc)
	submissionsSvc.SetStore(store)
	contributionsSvc := contributions.NewService(contributions.NewRepository(pool), projectsSvc)
	recoveryCtx, recoveryCancel := context.WithCancel(rootCtx)
	recoveryDone := make(chan struct{})
	defer func() { recoveryCancel(); <-recoveryDone }()
	go func() {
		defer close(recoveryDone)
		tick := time.NewTicker(time.Hour)
		defer tick.Stop()
		for {
			ctx, cancel := context.WithTimeout(recoveryCtx, 30*time.Second)
			e := submissionsSvc.Sweep(ctx, time.Now().UTC())
			cancel()
			if e != nil && recoveryCtx.Err() == nil {
				logger.Warn("attachment recovery will retry")
			}
			select {
			case <-recoveryCtx.Done():
				return
			case <-tick.C:
			}
		}
	}()
	server := httpapi.New(cfg, logger, system.NewRepository(pool), authSvc, authHandler, teamsHandler, projectsHandler, tasksHandler, submissions.NewHandler(submissionsSvc, logger), contributions.NewHandler(contributionsSvc, logger))

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve http: %w", err)
	case <-rootCtx.Done():
		logger.Info("shutdown signal received", "timeout", cfg.ShutdownTimeout.String())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("nexora-api stopped")
	return nil
}

func migrateOnStart(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	db, err := database.OpenSQL(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database for migrations: %w", err)
	}
	defer db.Close()

	migrateCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := database.Up(migrateCtx, db, logger); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
