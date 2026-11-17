// Package teams implements team tenancy: per-team memberships with
// independent roles, controlled invitations and the authorization chain for
// team-scoped resources. Membership is always resolved per request against
// PostgreSQL; it is never cached in the session identity.
package teams

import (
	"time"

	"github.com/google/uuid"
)

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"

	StatusActive  = "active"
	StatusRemoved = "removed"

	InvitationPending   = "pending"
	InvitationAccepted  = "accepted"
	InvitationDeclined  = "declined"
	InvitationCancelled = "cancelled"

	DefaultInvitationTTLHours = 72
	MinInvitationTTLHours     = 1
	MaxInvitationTTLHours     = 168

	maxTeamNameRunes        = 64
	maxTeamDescriptionRunes = 500
)

// Team is the tenant boundary. Every team-owned resource references Team.ID.
type Team struct {
	ID          uuid.UUID
	Name        string
	Description string
	CreatedBy   uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TeamSummary is a team plus the requesting user's own role in it.
type TeamSummary struct {
	Team
	MyRole string
}

// Member is an active membership joined with its account display fields.
type Member struct {
	TeamID      uuid.UUID
	UserID      uuid.UUID
	Username    string
	DisplayName string
	Role        string
	JoinedAt    time.Time
}

// Invitation is a controlled join offer for an existing account.
type Invitation struct {
	ID              uuid.UUID
	TeamID          uuid.UUID
	TeamName        string
	InvitedUserID   uuid.UUID
	InvitedUsername string
	InvitedBy       uuid.UUID
	Role            string
	Status          string
	ExpiresAt       time.Time
	CreatedAt       time.Time
}

// Permissions is the capability view of one membership, derived strictly from
// the approved team permission matrix. Team roles never imply platform roles.
type Permissions struct {
	TeamID               uuid.UUID
	Role                 string
	CanInvite            bool
	CanManageInvitations bool
	CanRemoveMembers     bool
	CanChangeRoles       bool
	CanLeave             bool
}

func permissionsFor(teamID uuid.UUID, role string) Permissions {
	manager := role == RoleOwner || role == RoleAdmin
	return Permissions{
		TeamID:               teamID,
		Role:                 role,
		CanInvite:            manager,
		CanManageInvitations: manager,
		CanRemoveMembers:     manager,
		CanChangeRoles:       manager,
		CanLeave:             true,
	}
}

func validTeamRole(role string) bool {
	return role == RoleOwner || role == RoleAdmin || role == RoleMember
}

func validInvitationRole(role string) bool {
	return role == RoleAdmin || role == RoleMember
}
