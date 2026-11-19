package bonusrounds

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// The access CTE deliberately requires a live session plus active Team and
// Project membership in the same statement snapshot as the data. A removed
// member, an expired/revoked session or a cross-Team ID therefore returns no
// rows instead of a stale or partial grant. Archived projects remain readable
// because the bonus domain has its own lifecycle (D9/B).
const roundAccess = `access AS (
	SELECT EXISTS(
		SELECT 1
		FROM sessions s
		JOIN team_members tm ON tm.team_id=$1 AND tm.user_id=s.user_id AND tm.status='active'
		JOIN project_members pm ON pm.team_id=$1 AND pm.project_id=$2 AND pm.user_id=s.user_id AND pm.status='active'
		WHERE s.id=$4 AND s.user_id=$3
		  AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp()
	) AS allowed
)`

const roundVersionJSON = `jsonb_build_object(
	'version', %[1]s.version,
	'status', %[1]s.status,
	'created_at', %[1]s.created_at,
	'submitted_at', %[1]s.submitted_at,
	'confirmed_at', %[2]s.confirmed_at,
	'effective', %[3]s
)`

func versionJSON(alias, confirmationAlias string, effective string) string {
	return fmt.Sprintf(roundVersionJSON, alias, confirmationAlias, effective)
}

// List returns one row per funding round, never one row per plan version.
func (r *Repository) List(ctx context.Context, teamID, projectID, userID, sessionID uuid.UUID) (Page, error) {
	var allowed bool
	var raw []byte
	err := r.pool.QueryRow(ctx, `WITH `+roundAccess+`,
rounds AS (
	SELECT
		br.id,
		br.project_id,
		br.funding_key,
		br.funding_label,
		br.funding_source,
		br.revision,
		br.created_at,
		CASE WHEN lv.id IS NULL THEN NULL ELSE `+versionJSON("lv", "lc", "false")+` END AS latest_version,
		CASE WHEN ev.id IS NULL THEN NULL ELSE `+versionJSON("ev", "ec", "true")+` END AS effective_confirmed_version
	FROM access a
	JOIN bonus_rounds br ON a.allowed AND br.team_id=$1 AND br.project_id=$2
	LEFT JOIN bonus_round_versions lv
		ON lv.team_id=br.team_id AND lv.project_id=br.project_id
		AND lv.round_id=br.id AND lv.id=br.latest_version_id
	LEFT JOIN bonus_confirmations lc
		ON lc.team_id=br.team_id AND lc.project_id=br.project_id
		AND lc.round_id=br.id AND lc.version_id=lv.id
	LEFT JOIN bonus_round_versions ev
		ON ev.team_id=br.team_id AND ev.project_id=br.project_id
		AND ev.round_id=br.id AND ev.id=br.effective_confirmed_version_id
		AND ev.status='confirmed'
	LEFT JOIN bonus_confirmations ec
		ON ec.team_id=br.team_id AND ec.project_id=br.project_id
		AND ec.round_id=br.id AND ec.version_id=ev.id
	ORDER BY br.created_at DESC, br.id DESC
	LIMIT 101
)
SELECT a.allowed, COALESCE(
	(SELECT jsonb_agg(to_jsonb(r) ORDER BY r.created_at DESC,r.id DESC) FROM rounds r),
	'[]'::jsonb
)
FROM access a`, teamID, projectID, userID, sessionID).Scan(&allowed, &raw)
	if err != nil {
		return Page{}, fmt.Errorf("query bonus rounds: %w", err)
	}
	if !allowed {
		return Page{}, ErrNotFound
	}
	var items []Summary
	if err := json.Unmarshal(raw, &items); err != nil {
		return Page{}, fmt.Errorf("decode bonus rounds: %w", err)
	}
	if items == nil {
		items = []Summary{}
	}
	for i := range items {
		items[i].Status = deriveStatus(items[i].LatestVersion, items[i].EffectiveConfirmedVersion)
	}
	more := len(items) > 100
	if more {
		items = items[:100]
	}
	return Page{Items: items, HasMore: more}, nil
}

// Get keeps the same access predicate as List. A round from another Team or
// Project is indistinguishable from a missing round.
func (r *Repository) Get(ctx context.Context, teamID, projectID, roundID, userID, sessionID uuid.UUID) (Detail, error) {
	var allowed bool
	var roundRaw, versionsRaw []byte
	err := r.pool.QueryRow(ctx, `WITH `+roundAccess+`,
round_row AS (
	SELECT
		br.id,
		br.project_id,
		br.funding_key,
		br.funding_label,
		br.funding_source,
		br.revision,
		br.created_at,
		br.effective_confirmed_version_id,
		CASE WHEN lv.id IS NULL THEN NULL ELSE `+versionJSON("lv", "lc", "false")+` END AS latest_version,
		CASE WHEN ev.id IS NULL THEN NULL ELSE `+versionJSON("ev", "ec", "true")+` END AS effective_confirmed_version
	FROM access a
	JOIN bonus_rounds br ON a.allowed AND br.team_id=$1 AND br.project_id=$2 AND br.id=$5
	LEFT JOIN bonus_round_versions lv
		ON lv.team_id=br.team_id AND lv.project_id=br.project_id
		AND lv.round_id=br.id AND lv.id=br.latest_version_id
	LEFT JOIN bonus_confirmations lc
		ON lc.team_id=br.team_id AND lc.project_id=br.project_id
		AND lc.round_id=br.id AND lc.version_id=lv.id
	LEFT JOIN bonus_round_versions ev
		ON ev.team_id=br.team_id AND ev.project_id=br.project_id
		AND ev.round_id=br.id AND ev.id=br.effective_confirmed_version_id
		AND ev.status='confirmed'
	LEFT JOIN bonus_confirmations ec
		ON ec.team_id=br.team_id AND ec.project_id=br.project_id
		AND ec.round_id=br.id AND ec.version_id=ev.id
),
versions AS (
	SELECT
		v.version,
		v.status,
		v.created_at,
		v.submitted_at,
		c.confirmed_at,
		(v.id=rr.effective_confirmed_version_id AND v.status='confirmed') AS effective
	FROM round_row rr
	JOIN bonus_round_versions v
		ON v.team_id=$1 AND v.project_id=$2 AND v.round_id=rr.id
	LEFT JOIN bonus_confirmations c
		ON c.team_id=$1 AND c.project_id=$2 AND c.round_id=rr.id AND c.version_id=v.id
	ORDER BY v.version DESC
)
SELECT a.allowed,
	(SELECT to_jsonb(rr) FROM round_row rr),
	COALESCE((SELECT jsonb_agg(to_jsonb(v) ORDER BY v.version DESC) FROM versions v),'[]'::jsonb)
FROM access a`, teamID, projectID, userID, sessionID, roundID).Scan(&allowed, &roundRaw, &versionsRaw)
	if err != nil {
		return Detail{}, fmt.Errorf("query bonus round: %w", err)
	}
	if !allowed || roundRaw == nil {
		return Detail{}, ErrNotFound
	}
	var out Detail
	if err := json.Unmarshal(roundRaw, &out.Summary); err != nil {
		return Detail{}, fmt.Errorf("decode bonus round: %w", err)
	}
	if err := json.Unmarshal(versionsRaw, &out.Versions); err != nil {
		return Detail{}, fmt.Errorf("decode bonus round versions: %w", err)
	}
	if out.Versions == nil {
		out.Versions = []Version{}
	}
	out.Status = deriveStatus(out.LatestVersion, out.EffectiveConfirmedVersion)
	out.AmountsVisible = false
	return out, nil
}
