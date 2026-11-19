package bonusresults

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Read subject-only results in ONE statement/snapshot. Membership, live
// session, current effective version and allocation filters must all hold.
// This deliberately never falls back to ROOT or team administrator access.
func (r *Repository) MyResults(ctx context.Context, teamID, userID, sessionID uuid.UUID) (Page, error) {
	var allowed bool
	var raw []byte
	if err := r.pool.QueryRow(ctx, `WITH access AS (
	SELECT EXISTS(
		SELECT 1
		FROM sessions s
		JOIN team_members tm ON tm.user_id=s.user_id AND tm.team_id=$1 AND tm.status='active'
		WHERE s.id=$3 AND s.user_id=$2
		  AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp()
		  AND EXISTS(
			SELECT 1 FROM project_members pm
			WHERE pm.team_id=$1 AND pm.user_id=$2 AND pm.status='active'
		  )
	) AS allowed
),
results AS (
	SELECT
		br.id AS round_id,
		br.project_id,
		br.funding_label,
		bv.currency_code,
		bv.currency_minor_unit,
		totals.total_minor::bigint AS amount_minor,
		'confirmed'::text AS status,
		'not_recorded'::text AS payment_status,
		bc.confirmed_at
	FROM access a
	JOIN bonus_rounds br ON a.allowed AND br.team_id=$1
	JOIN bonus_round_versions bv ON bv.id=br.effective_confirmed_version_id
		AND bv.team_id=br.team_id AND bv.project_id=br.project_id
		AND bv.round_id=br.id AND bv.status='confirmed'
	JOIN bonus_confirmations bc ON bc.version_id=bv.id
		AND bc.team_id=br.team_id AND bc.project_id=br.project_id AND bc.round_id=br.id
	JOIN bonus_member_totals totals ON totals.team_id=br.team_id
		AND totals.project_id=br.project_id AND totals.round_id=br.id
		AND totals.version_id=bv.id AND totals.user_id=$2
	JOIN project_members pm ON pm.team_id=br.team_id AND pm.project_id=br.project_id
		AND pm.user_id=$2 AND pm.status='active'
	ORDER BY bc.confirmed_at DESC, br.id DESC
	LIMIT 101
)
SELECT a.allowed, COALESCE(
	(SELECT jsonb_agg(to_jsonb(r) ORDER BY r.confirmed_at DESC,r.round_id DESC) FROM results r),
	'[]'::jsonb
)
FROM access a`, teamID, userID, sessionID).Scan(&allowed, &raw); err != nil {
		return Page{}, fmt.Errorf("query own bonus results: %w", err)
	}
	if !allowed {
		return Page{}, ErrNotMember
	}
	var items []Result
	if err := json.Unmarshal(raw, &items); err != nil {
		return Page{}, fmt.Errorf("decode own bonus results: %w", err)
	}
	if items == nil {
		items = []Result{}
	}
	more := len(items) > 100
	if more {
		items = items[:100]
	}
	return Page{Items: items, HasMore: more}, nil
}
