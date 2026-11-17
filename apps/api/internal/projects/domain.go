// Package projects owns project resources and the tenant authorization boundary
// reused by tasks. No platform role implicitly grants project management.
package projects

import (
	"time"

	"github.com/google/uuid"
)

type Project struct {
	ID             uuid.UUID  `json:"id"`
	TeamID         uuid.UUID  `json:"team_id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	LeadID         uuid.UUID  `json:"lead_id"`
	Status         string     `json:"status"`
	PlannedStartAt *time.Time `json:"planned_start_at"`
	PlannedEndAt   *time.Time `json:"planned_end_at"`
	ArchivedAt     *time.Time `json:"archived_at"`
	Version        int        `json:"version"`
	CreatedBy      uuid.UUID  `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	CanManage      bool       `json:"can_manage"`
	CanHandoff     bool       `json:"can_handoff"`
}
type ProjectInput struct {
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	PlannedStartAt *time.Time `json:"planned_start_at"`
	PlannedEndAt   *time.Time `json:"planned_end_at"`
}
type CreateInput struct {
	ProjectInput
	LeadID uuid.UUID `json:"lead_id"`
}
type EditInput struct {
	ProjectInput
	ExpectedVersion int `json:"expected_version"`
}
type Member struct {
	UserID      uuid.UUID `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	Active      bool      `json:"active"`
}
type Milestone struct {
	ID          uuid.UUID  `json:"id"`
	TeamID      uuid.UUID  `json:"team_id"`
	ProjectID   uuid.UUID  `json:"project_id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	PlannedAt   *time.Time `json:"planned_at"`
	Version     int        `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
type MilestoneInput struct {
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	PlannedAt       *time.Time `json:"planned_at"`
	ExpectedVersion int        `json:"expected_version"`
}
