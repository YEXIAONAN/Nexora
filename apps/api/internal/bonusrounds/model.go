// Package bonusrounds exposes read-only, project-scoped funding-round
// metadata. It deliberately has no create, budget, eligibility or adjustment
// command: those writes stay blocked until the M1 authorization model and the
// M2 database gates are explicitly approved and replaced by a later migration.
package bonusrounds

import (
	"time"

	"github.com/google/uuid"
)

// Round workflow states are derived from the immutable version facts and the
// two round pointers. They never overwrite a version status.
const (
	StatusDraft         = "draft"
	StatusPendingReview = "pending_review"
	StatusConfirmed     = "confirmed"
	StatusRevising      = "revising"
	StatusAbandoned     = "abandoned"
	StatusUnavailable   = "unavailable"
)

// Version is a plan-version summary. Budget and member amounts are absent by
// design: project read access is not bonus-amount access.
type Version struct {
	Version     int        `json:"version"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	SubmittedAt *time.Time `json:"submitted_at"`
	ConfirmedAt *time.Time `json:"confirmed_at"`
	Effective   bool       `json:"effective"`
}

// Summary is the non-monetary funding-round view shared by list and detail.
type Summary struct {
	ID                        uuid.UUID `json:"id"`
	ProjectID                 uuid.UUID `json:"project_id"`
	FundingKey                string    `json:"funding_key"`
	FundingLabel              string    `json:"funding_label"`
	FundingSource             string    `json:"funding_source"`
	Status                    string    `json:"status"`
	Revision                  int       `json:"revision"`
	CreatedAt                 time.Time `json:"created_at"`
	LatestVersion             *Version  `json:"latest_version"`
	EffectiveConfirmedVersion *Version  `json:"effective_confirmed_version"`
	// AmountsVisible is always false in this phase. It lets a client
	// distinguish "amount withheld" from "confirmed zero amount".
	AmountsVisible bool `json:"amounts_visible"`
}

// Detail adds the immutable version history to one round.
type Detail struct {
	Summary
	Versions []Version `json:"versions"`
}

type Page struct {
	Items   []Summary `json:"items"`
	HasMore bool      `json:"has_more"`
}
