package bonusrounds

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

var ErrNotFound = errors.New("bonus round is not visible to the current subject")

type Reader interface {
	List(ctx context.Context, teamID, projectID, userID, sessionID uuid.UUID) (Page, error)
	Get(ctx context.Context, teamID, projectID, roundID, userID, sessionID uuid.UUID) (Detail, error)
}

type Service struct{ reader Reader }

func NewService(reader Reader) *Service { return &Service{reader: reader} }

func authenticate(identity reqctx.Identity) error {
	if identity.UserID == uuid.Nil || identity.SessionID == uuid.Nil {
		return httpx.Unauthorized("authentication required")
	}
	return nil
}

// List returns only the rounds of the requested Team and Project. Access is
// resolved server-side from the session subject; the caller cannot name another
// member or widen the scope. Amounts stay hidden for every role in this phase.
func (s *Service) List(ctx context.Context, identity reqctx.Identity, teamID, projectID uuid.UUID) (Page, error) {
	empty := Page{Items: []Summary{}}
	if err := authenticate(identity); err != nil {
		return empty, err
	}
	if teamID == uuid.Nil || projectID == uuid.Nil {
		return empty, httpx.BadRequest("team id and project id are required")
	}
	out, err := s.reader.List(ctx, teamID, projectID, identity.UserID, identity.SessionID)
	if errors.Is(err, ErrNotFound) {
		return empty, httpx.NotFound("the requested resource does not exist")
	}
	if err != nil {
		return empty, httpx.Internal(err)
	}
	if out.Items == nil {
		out.Items = []Summary{}
	}
	for i := range out.Items {
		// Defense in depth: a repository must never make money visible here.
		out.Items[i].AmountsVisible = false
	}
	return out, nil
}

// Get returns one round only when it belongs to the requested Team and Project.
// Team owner, project lead and ROOT are ordinary subjects here.
func (s *Service) Get(ctx context.Context, identity reqctx.Identity, teamID, projectID, roundID uuid.UUID) (Detail, error) {
	empty := Detail{Summary: Summary{AmountsVisible: false}, Versions: []Version{}}
	if err := authenticate(identity); err != nil {
		return empty, err
	}
	if teamID == uuid.Nil || projectID == uuid.Nil || roundID == uuid.Nil {
		return empty, httpx.BadRequest("team id, project id and round id are required")
	}
	out, err := s.reader.Get(ctx, teamID, projectID, roundID, identity.UserID, identity.SessionID)
	if errors.Is(err, ErrNotFound) {
		return empty, httpx.NotFound("the requested resource does not exist")
	}
	if err != nil {
		return empty, httpx.Internal(err)
	}
	out.AmountsVisible = false
	if out.Versions == nil {
		out.Versions = []Version{}
	}
	return out, nil
}

// deriveStatus follows the Phase06 M1 precedence without inventing a new
// version state: an effective confirmation wins, a newer working version means
// revising, and any pointer/status inconsistency fails closed as unavailable.
func deriveStatus(latest, effective *Version) string {
	if effective != nil {
		if latest == nil || latest.Version == effective.Version {
			return StatusConfirmed
		}
		switch latest.Status {
		case "draft", "pending_review":
			// A newer working version does not replace the effective result.
			return StatusRevising
		case "abandoned":
			// An abandoned revision leaves the prior formal result in force.
			return StatusConfirmed
		default:
			// A newer confirmed/superseded/overturned version without a moved
			// effective pointer is inconsistent and must not be presented.
			return StatusUnavailable
		}
	}
	if latest == nil {
		return StatusDraft
	}
	switch latest.Status {
	case "draft":
		return StatusDraft
	case "pending_review":
		return StatusPendingReview
	case "abandoned":
		return StatusAbandoned
	default:
		// confirmed/superseded/overturned without an effective pointer cannot
		// be presented as a formal result.
		return StatusUnavailable
	}
}
