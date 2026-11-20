package bonusresults

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

// Confirmed is the only amount state exposed by this phase. Payment is not
// executed or recorded by Nexora, so a result can never claim it was paid.
const (
	StatusConfirmed          = "confirmed"
	PaymentStatusNotRecorded = "not_recorded"
)

// Result contains only the authenticated subject's currently effective,
// independently confirmed allocation. No other member or pool totals appear.
// Status and PaymentStatus are deliberately separate: an internally confirmed
// amount is not evidence of an actual payment.
type Result struct {
	RoundID           uuid.UUID `json:"round_id"`
	ProjectID         uuid.UUID `json:"project_id"`
	FundingLabel      string    `json:"funding_label"`
	CurrencyCode      string    `json:"currency_code"`
	CurrencyMinorUnit int16     `json:"currency_minor_unit"`
	AmountMinor       int64     `json:"amount_minor"`
	Status            string    `json:"status"`
	PaymentStatus     string    `json:"payment_status"`
	ConfirmedAt       time.Time `json:"confirmed_at"`
}

type Page struct {
	Items   []Result `json:"items"`
	HasMore bool     `json:"has_more"`
}

var ErrNotMember = errors.New("current team access is required")

type Reader interface {
	MyResults(ctx context.Context, teamID, userID, sessionID uuid.UUID) (Page, error)
}

type Service struct{ reader Reader }

func NewService(reader Reader) *Service { return &Service{reader: reader} }

// Mine is deliberately read-only. Schema v8 blocks formal confirmations
// until the business policy is approved, so an initial empty list is expected.
func (s *Service) Mine(ctx context.Context, identity reqctx.Identity, teamID uuid.UUID) (Page, error) {
	empty := Page{Items: []Result{}}
	if identity.UserID == uuid.Nil || identity.SessionID == uuid.Nil {
		return empty, httpx.Unauthorized("authentication required")
	}
	if teamID == uuid.Nil {
		return empty, httpx.BadRequest("team id is required")
	}
	out, err := s.reader.MyResults(ctx, teamID, identity.UserID, identity.SessionID)
	if errors.Is(err, ErrNotMember) {
		return empty, httpx.NotFound("team not found")
	}
	if err != nil {
		return empty, httpx.Internal(err)
	}
	if out.Items == nil {
		out.Items = []Result{}
	}
	return out, nil
}
