// Command migrate applies, inspects or rolls back database migrations.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/config"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/database"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/observability"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "nexora-migrate: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	command := flag.String("command", "status", "migration command: up, down, status or version")
	steps := flag.Int("steps", 1, "number of migrations to roll back when command=down")
	allowDestructive := flag.Bool("allow-destructive", false, "explicitly allow a rollback in production")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	logger := observability.NewLogger(cfg.LogLevel)

	db, err := database.OpenSQL(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect to database: %w", database.SafeError(err))
	}

	switch *command {
	case "up":
		return database.Up(ctx, db, logger)
	case "down":
		if cfg.Environment == config.EnvironmentProduction && !*allowDestructive {
			return errors.New("refusing to roll back in production without -allow-destructive")
		}
		return database.Down(ctx, db, *steps, logger)
	case "status":
		return database.Status(ctx, db, logger)
	case "version":
		version, err := database.Version(ctx, db)
		if err != nil {
			return err
		}
		fmt.Printf("schema version: %d\n", version)
		return nil
	default:
		return fmt.Errorf("unknown command %q (expected up, down, status or version)", *command)
	}
}
