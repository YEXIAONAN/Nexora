package teams

import "errors"

var (
	// ErrNotFound covers missing teams, memberships and invitations. It is
	// also returned to non-members so team existence is never leaked.
	ErrNotFound = errors.New("teams: not found")
	// ErrLastOwner is raised by the database guard when a mutation would
	// leave the team without any active owner.
	ErrProjectLead = errors.New("teams: project lead requires handoff")
	ErrLastOwner   = errors.New("teams: last active owner")
	// ErrDuplicateInvitation means a pending invitation already exists for
	// the same team and user.
	ErrDuplicateInvitation = errors.New("teams: duplicate pending invitation")
)
