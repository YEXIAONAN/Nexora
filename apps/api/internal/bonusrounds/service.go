package bonusrounds

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

var ErrNotFound = errors.New("bonus round is not visible to the current subject")

type Reader interface {
	List(ctx context.Context, teamID, projectID, userID, sessionID uuid.UUID) (Page, error)
	Get(ctx context.Context, teamID, projectID, roundID, userID, sessionID uuid.UUID) (Detail, error)
}

type WorkflowStore interface {
	ProjectWorkspace(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (ProjectWorkspace, error)
	Workspace(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (Workspace, error)
	Execute(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, string, any) (CommandResult, error)
}

type Service struct{ reader Reader }

func workflowError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return httpx.NotFound("the requested resource does not exist")
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "P0002":
			return httpx.NotFound("the requested resource does not exist")
		case "42501":
			return httpx.Forbidden("explicit current bonus authority and independent review are required")
		case "23505", "40001", "23514", "23503":
			return httpx.Conflict("bonus state, evidence, authorization or revision changed; refresh and review the plan")
		case "22023", "22P02", "22003", "23502":
			return httpx.BadRequest("invalid bonus command fields")
		}
	}
	return httpx.Internal(err)
}

func (s *Service) ProjectWorkspace(ctx context.Context, identity reqctx.Identity, teamID, projectID uuid.UUID) (ProjectWorkspace, error) {
	if err := authenticate(identity); err != nil {
		return ProjectWorkspace{}, err
	}
	if teamID == uuid.Nil || projectID == uuid.Nil {
		return ProjectWorkspace{}, httpx.BadRequest("team and project are required")
	}
	store, ok := s.reader.(WorkflowStore)
	if !ok {
		return ProjectWorkspace{}, httpx.Unavailable("bonus workflow is not configured")
	}
	out, err := store.ProjectWorkspace(ctx, teamID, projectID, identity.UserID, identity.SessionID)
	return out, workflowError(err)
}

func (s *Service) Workspace(ctx context.Context, identity reqctx.Identity, teamID, projectID, roundID uuid.UUID) (Workspace, error) {
	if err := authenticate(identity); err != nil {
		return Workspace{}, err
	}
	if teamID == uuid.Nil || projectID == uuid.Nil || roundID == uuid.Nil {
		return Workspace{}, httpx.BadRequest("team, project and round are required")
	}
	store, ok := s.reader.(WorkflowStore)
	if !ok {
		return Workspace{}, httpx.Unavailable("bonus workflow is not configured")
	}
	out, err := store.Workspace(ctx, teamID, projectID, roundID, identity.UserID, identity.SessionID)
	return out, workflowError(err)
}

func boundedText(value string, max int) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(value))
	return n > 0 && n <= max
}
func validMoney(n int64) bool { return n >= 0 && n <= MaxAmountMinor }

var fundingKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,119}$`)

// Validate before reaching storage; the security function repeats business
// validation so direct EXECUTE cannot bypass the same rules.
func validateWorkflow(action string, input any, actor uuid.UUID) error {
	invalid := httpx.BadRequest("invalid bonus command fields")
	var command Command
	switch in := input.(type) {
	case CreateInput:
		if action != "create" || !fundingKeyPattern.MatchString(in.FundingKey) || !boundedText(in.FundingLabel, 200) || !boundedText(in.FundingSource, 400) || !boundedText(in.Reason, 4000) || in.ClientRequestID == uuid.Nil || in.ManagerID == uuid.Nil || in.ReviewerID == uuid.Nil || in.ManagerID == in.ReviewerID || in.ManagerID == actor || in.ReviewerID == actor {
			return invalid
		}
		return nil
	case PlanInput:
		if action != "plan" || !validMoney(in.TotalBudgetMinor) || in.Sources == nil || len(in.Sources) > 200 {
			return invalid
		}
		command = in.Command
		var budgets int64
		sources := map[uuid.UUID]bool{}
		eligibility := map[uuid.UUID]bool{}
		for _, src := range in.Sources {
			if src.ConfirmationID == uuid.Nil || sources[src.ConfirmationID] || !validMoney(src.SourceBudgetMinor) || !boundedText(src.Reason, 4000) || len(src.Lines) == 0 || len(src.Lines) > 500 {
				return invalid
			}
			sources[src.ConfirmationID] = true
			if src.SourceBudgetMinor > in.TotalBudgetMinor-budgets {
				return invalid
			}
			budgets += src.SourceBudgetMinor
			var allocated int64
			users := map[uuid.UUID]bool{}
			for _, line := range src.Lines {
				if line.UserID == uuid.Nil || line.UserID == actor || users[line.UserID] || line.Eligible == nil || !validMoney(line.AmountMinor) || !boundedText(line.Reason, 4000) {
					return invalid
				}
				users[line.UserID] = true
				if !*line.Eligible && line.AmountMinor != 0 {
					return invalid
				}
				if previous, exists := eligibility[line.UserID]; exists && previous != *line.Eligible {
					return invalid
				}
				eligibility[line.UserID] = *line.Eligible
				if line.AmountMinor > src.SourceBudgetMinor-allocated {
					return invalid
				}
				allocated += line.AmountMinor
			}
		}
	case PaymentInput:
		if action != "payments" || in.UserID == uuid.Nil || !boundedText(in.Reference, 200) {
			return invalid
		}
		command = in.Command
	case GrantInput:
		if action != "grants" || in.SubjectID == uuid.Nil || in.SubjectID == actor {
			return invalid
		}
		switch in.RoleCode {
		case "editor", "budget", "eligibility", "confirmer", "auditor":
		default:
			return invalid
		}
		command = in.Command
	case Command:
		switch action {
		case "submit", "return", "confirm", "publish", "revise":
		default:
			return invalid
		}
		command = in
	default:
		return invalid
	}
	if command.ExpectedRevision <= 0 || command.ExpectedRevision > 2147483647 || command.ClientRequestID == uuid.Nil || !boundedText(command.Reason, 4000) {
		return invalid
	}
	return nil
}

func (s *Service) Execute(ctx context.Context, identity reqctx.Identity, teamID, projectID, roundID uuid.UUID, action string, input any) (CommandResult, error) {
	if err := authenticate(identity); err != nil {
		return CommandResult{}, err
	}
	if teamID == uuid.Nil || projectID == uuid.Nil || (action != "create" && roundID == uuid.Nil) || (action == "create" && roundID != uuid.Nil) {
		return CommandResult{}, httpx.BadRequest("invalid bonus command scope")
	}
	if err := validateWorkflow(action, input, identity.UserID); err != nil {
		return CommandResult{}, err
	}
	store, ok := s.reader.(WorkflowStore)
	if !ok {
		return CommandResult{}, httpx.Unavailable("bonus workflow is not configured")
	}
	out, err := store.Execute(ctx, teamID, projectID, roundID, identity.UserID, identity.SessionID, action, input)
	return out, workflowError(err)
}

func NewService(reader Reader) *Service { return &Service{reader: reader} }

func authenticate(identity reqctx.Identity) error {
	if identity.UserID == uuid.Nil || identity.SessionID == uuid.Nil {
		return httpx.Unauthorized("authentication required")
	}
	return nil
}

// List returns only the rounds of the requested Team and Project. Access is
// resolved server-side from the session subject; the caller cannot name another
// member or widen the scope. Metadata never includes amounts, for any role.
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
