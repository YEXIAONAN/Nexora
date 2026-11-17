// Package audit writes append-only operation audit events. Events are written
// inside the caller's business transaction so that a permission change and its
// audit record commit or roll back together.
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Event describes one audited operation. Details must never contain passwords,
// cookies or raw session tokens.
type Event struct {
	ActorID    *uuid.UUID
	Action     string
	TargetType string
	TargetID   *uuid.UUID
	TeamID     *uuid.UUID
	RequestID  string
	Details    map[string]any
}

// Repository inserts audit events into audit_events.
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Insert writes the event within tx. The database trigger forbids any later
// UPDATE or DELETE of the row.
func (r *Repository) Insert(ctx context.Context, tx pgx.Tx, ev Event) error {
	details := ev.Details
	if details == nil {
		details = map[string]any{}
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode audit details: %w", err)
	}
	var targetType any
	if ev.TargetType != "" {
		targetType = ev.TargetType
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (actor_id, action, target_type, target_id, team_id, request_id, details)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		ev.ActorID, ev.Action, targetType, ev.TargetID, ev.TeamID, ev.RequestID, raw)
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

// InsertStandalone writes an event in its own transaction. It is only for
// events without an accompanying business change, such as failed logins.
func (r *Repository) InsertStandalone(ctx context.Context, ev Event) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin audit transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op
	if err := r.Insert(ctx, tx, ev); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit audit transaction: %w", err)
	}
	return nil
}
