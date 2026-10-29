package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const migrationsDir = "migrations"

type gooseLogger struct {
	logger *slog.Logger
}

func (l *gooseLogger) Printf(format string, v ...any) {
	l.logger.Info(fmt.Sprintf(format, v...))
}

func (l *gooseLogger) Fatalf(format string, v ...any) {
	message := fmt.Sprintf(format, v...)
	l.logger.Error(message)
	panic(message)
}

func configure(logger *slog.Logger) error {
	goose.SetBaseFS(migrationFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("configure migration dialect: %w", err)
	}
	goose.SetLogger(&gooseLogger{logger: logger})
	return nil
}

// Up applies every pending migration.
func Up(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	if err := configure(logger); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// Down rolls back at most steps migrations and refuses to go below zero.
func Down(ctx context.Context, db *sql.DB, steps int, logger *slog.Logger) error {
	if steps < 1 {
		return fmt.Errorf("steps must be at least 1")
	}
	if err := configure(logger); err != nil {
		return err
	}
	for range steps {
		version, err := goose.GetDBVersionContext(ctx, db)
		if err != nil {
			return fmt.Errorf("read schema version: %w", err)
		}
		if version == 0 {
			return nil
		}
		if err := goose.DownContext(ctx, db, migrationsDir); err != nil {
			return fmt.Errorf("roll back migration %d: %w", version, err)
		}
	}
	return nil
}

// Status prints the migration status of the connected database.
func Status(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	if err := configure(logger); err != nil {
		return err
	}
	if err := goose.StatusContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("read migration status: %w", err)
	}
	return nil
}

// Version returns the currently applied schema version.
func Version(ctx context.Context, db *sql.DB) (int64, error) {
	if err := configure(slog.Default()); err != nil {
		return 0, err
	}
	version, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}
