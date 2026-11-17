// Package database owns PostgreSQL connectivity and schema migrations.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/config"
)

// Open creates the application connection pool and verifies connectivity.
func Open(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database configuration: %w", SafeError(err))
	}
	poolConfig.MaxConns = cfg.DBMaxConns
	poolConfig.MinConns = cfg.DBMinConns
	poolConfig.MaxConnLifetime = cfg.DBMaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.DBMaxConnIdleTime
	poolConfig.ConnConfig.RuntimeParams["application_name"] = "nexora-api"
	poolConfig.ConnConfig.RuntimeParams["timezone"] = "UTC"

	connectCtx, cancel := context.WithTimeout(ctx, cfg.DBConnectTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(connectCtx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", SafeError(err))
	}
	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", SafeError(err))
	}
	return pool, nil
}

// OpenSQL opens a database/sql handle used by the migration runner. The
// application runtime uses the pgx pool directly.
func OpenSQL(dsn string) (*sql.DB, error) {
	// database/sql otherwise delays URL parsing until the first connection,
	// where a raw ParseConfigError could escape through Ping or goose.
	connConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", SafeError(err))
	}
	db := stdlib.OpenDB(*connConfig)
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)
	return db, nil
}
