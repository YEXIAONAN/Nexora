// Package bonusrounds exposes project-scoped metadata and explicitly authorized,
// independently reviewed manual bonus allocation. Payment records are simulation
// bookkeeping, not execution of a real payment.
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
	// AmountsVisible is false for public metadata and true only inside an
	// explicitly authorized workspace; withheld is not a confirmed zero.
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

const MaxAmountMinor int64 = 900000000000000

type Member struct {
	UserID      uuid.UUID `json:"user_id"`
	DisplayName string    `json:"display_name"`
}
type SourceShare struct {
	Member
	ShareBPS int `json:"share_bps"`
}
type AvailableSource struct {
	ConfirmationID uuid.UUID     `json:"confirmation_id"`
	CaseVersionID  uuid.UUID     `json:"case_version_id"`
	TaskTitle      string        `json:"task_title"`
	Shares         []SourceShare `json:"shares"`
}
type ProjectWorkspace struct {
	CanCreate bool              `json:"can_create"`
	Members   []Member          `json:"members"`
	Sources   []AvailableSource `json:"sources"`
}
type PlanLine struct {
	SourceShare
	Eligible    bool   `json:"eligible"`
	AmountMinor int64  `json:"amount_minor"`
	Reason      string `json:"reason"`
}
type PlanSource struct {
	ConfirmationID         uuid.UUID  `json:"confirmation_id"`
	TaskTitle              string     `json:"task_title"`
	SourceBudgetMinor      int64      `json:"source_budget_minor"`
	SourceUnallocatedMinor int64      `json:"source_unallocated_minor"`
	Reason                 string     `json:"reason"`
	Lines                  []PlanLine `json:"lines"`
}
type Plan struct {
	ID                   uuid.UUID    `json:"id"`
	Version              int          `json:"version"`
	Status               string       `json:"status"`
	TotalBudgetMinor     int64        `json:"total_budget_minor"`
	PlanUnallocatedMinor int64        `json:"plan_unallocated_minor"`
	Rationale            string       `json:"rationale"`
	PublishedAt          *time.Time   `json:"published_at"`
	Sources              []PlanSource `json:"sources"`
}
type Permissions struct {
	Edit          bool `json:"edit"`
	Submit        bool `json:"submit"`
	Return        bool `json:"return"`
	Confirm       bool `json:"confirm"`
	Publish       bool `json:"publish"`
	Revise        bool `json:"revise"`
	RecordPayment bool `json:"record_payment"`
}
type Grant struct {
	SubjectID   uuid.UUID `json:"subject_id"`
	DisplayName string    `json:"display_name"`
	RoleCode    string    `json:"role_code"`
}
type Event struct {
	EventType string    `json:"event_type"`
	Reason    string    `json:"reason"`
	ActorID   uuid.UUID `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
}
type Payment struct {
	UserID      uuid.UUID `json:"user_id"`
	AmountMinor int64     `json:"amount_minor"`
	Reference   string    `json:"reference"`
	RecordedAt  time.Time `json:"recorded_at"`
}
type Workspace struct {
	Round       Summary     `json:"round"`
	Version     *Plan       `json:"version"`
	Permissions Permissions `json:"permissions"`
	Grants      []Grant     `json:"grants"`
	Events      []Event     `json:"events"`
	Payments    []Payment   `json:"payments"`
}
type Command struct {
	ExpectedRevision int       `json:"expected_revision"`
	ClientRequestID  uuid.UUID `json:"client_request_id"`
	Reason           string    `json:"reason"`
}
type CreateInput struct {
	FundingKey      string    `json:"funding_key"`
	FundingLabel    string    `json:"funding_label"`
	FundingSource   string    `json:"funding_source"`
	ManagerID       uuid.UUID `json:"manager_id"`
	ReviewerID      uuid.UUID `json:"reviewer_id"`
	Reason          string    `json:"reason"`
	ClientRequestID uuid.UUID `json:"client_request_id"`
}

// Eligible is a pointer to reject omitted/null decisions rather than silently
// treating them as explicit ineligibility. Money uses int64, never float64.
type PlanLineInput struct {
	UserID      uuid.UUID `json:"user_id"`
	Eligible    *bool     `json:"eligible"`
	AmountMinor int64     `json:"amount_minor"`
	Reason      string    `json:"reason"`
}
type PlanSourceInput struct {
	ConfirmationID    uuid.UUID       `json:"confirmation_id"`
	SourceBudgetMinor int64           `json:"source_budget_minor"`
	Reason            string          `json:"reason"`
	Lines             []PlanLineInput `json:"lines"`
}
type PlanInput struct {
	Command
	TotalBudgetMinor int64             `json:"total_budget_minor"`
	Sources          []PlanSourceInput `json:"sources"`
}
type PaymentInput struct {
	Command
	UserID    uuid.UUID `json:"user_id"`
	Reference string    `json:"reference"`
}
type GrantInput struct {
	Command
	SubjectID uuid.UUID `json:"subject_id"`
	RoleCode  string    `json:"role_code"`
	Revoke    bool      `json:"revoke"`
}
type CommandResult struct {
	RoundID  uuid.UUID `json:"round_id"`
	Revision int       `json:"revision"`
}
