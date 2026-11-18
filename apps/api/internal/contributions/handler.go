package contributions

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/auth"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/YEXIAONAN/Nexora/apps/api/internal/reqctx"
)

type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(s *Service, l *slog.Logger) *Handler { return &Handler{s, l} }
func (h *Handler) AuthenticatedRoutes(r chi.Router) {
	base := "/teams/{teamID}/projects/{projectID}"
	for _, v := range []struct{ method, path, action string }{
		{"GET", "/contribution-appraisers", "appraisers"}, {"POST", "/contribution-appraisers", "grant"}, {"DELETE", "/contribution-appraisers/{userID}", "revoke"},
		{"GET", "/contributions/eligibility", "eligibility"}, {"GET", "/contributions/pending", "pending"}, {"GET", "/contributions", "cases"}, {"POST", "/contributions", "create"},
		{"GET", "/contributions/{caseID}", "get"}, {"GET", "/contributions/{caseID}/versions", "versions"}, {"GET", "/contributions/{caseID}/history", "history"}, {"GET", "/contributions/{caseID}/evidence", "evidence"},
		{"PATCH", "/contributions/{caseID}/draft", "draft"}, {"PUT", "/contributions/{caseID}/shares/{userID}", "share"}, {"DELETE", "/contributions/{caseID}/shares/{userID}", "remove"},
		{"POST", "/contributions/{caseID}/submit", "submit"}, {"POST", "/contributions/{caseID}/withdraw", "withdraw"}, {"POST", "/contributions/{caseID}/abandon", "abandon"}, {"POST", "/contributions/{caseID}/confirm", "confirm"}, {"POST", "/contributions/{caseID}/revisions", "revisions"},
		{"GET", "/contributions/{caseID}/disputes", "disputes"}, {"POST", "/contributions/{caseID}/disputes", "dispute"}, {"POST", "/contributions/{caseID}/disputes/{disputeID}/resolutions", "resolve"},
		{"POST", "/contributions/{caseID}/conflicts", "conflict"}, {"GET", "/contributions/{caseID}/notices", "notices"}, {"POST", "/contributions/{caseID}/notices", "notice"}, {"POST", "/contributions/{caseID}/notices/{noticeID}/receipt", "receipt"},
		{"GET", "/contribution-results/mine", "mine"},
	} {
		r.MethodFunc(v.method, base+v.path, h.handle(v.action))
	}
	r.Post("/contribution-dispute-requests", h.handle("intake"))
}
func (h *Handler) handle(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, ok := reqctx.IdentityFrom(r.Context())
		if !ok {
			httpx.WriteError(w, r, h.logger, httpx.Unauthorized("authentication required"))
			return
		}
		ids := map[string]uuid.UUID{}
		for _, key := range []string{"teamID", "projectID", "caseID", "userID", "disputeID", "noticeID"} {
			if chi.URLParam(r, key) != "" {
				id, valid := httpx.ParseUUIDParam(w, r, h.logger, key)
				if !valid {
					return
				}
				ids[key] = id
			}
		}
		team, pid, cid := ids["teamID"], ids["projectID"], ids["caseID"]
		var out any
		var err error
		status := http.StatusOK
		switch action {
		case "appraisers":
			out, err = h.svc.Appraisers(r.Context(), a, team, pid)
		case "eligibility":
			out, err = h.svc.Eligibility(r.Context(), a, team, pid)
		case "cases", "pending":
			out, err = h.svc.List(r.Context(), a, team, pid, action == "pending")
		case "get":
			out, err = h.svc.Get(r.Context(), a, team, pid, cid)
		case "versions":
			out, err = h.svc.Versions(r.Context(), a, team, pid, cid)
		case "history":
			out, err = h.svc.History(r.Context(), a, team, pid, cid)
		case "evidence":
			out, err = h.svc.Evidence(r.Context(), a, team, pid, cid)
		case "disputes":
			out, err = h.svc.Disputes(r.Context(), a, team, pid, cid)
		case "notices":
			out, err = h.svc.Notices(r.Context(), a, team, pid, cid)
		case "mine":
			out, err = h.svc.MyResults(r.Context(), a, team, pid)
		case "grant":
			var in struct {
				AppraiserInput
				ExpectedVersion *int `json:"expected_version"`
			}
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			if in.ExpectedVersion == nil {
				err = invalid("expected_version", "must be explicitly provided (0 for the first grant)")
				break
			}
			in.AppraiserInput.Command.ExpectedVersion = *in.ExpectedVersion
			out, err = h.svc.SetAppraiser(r.Context(), a, team, pid, in.UserID, in.Command, false)
		case "revoke":
			var in Command
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			out, err = h.svc.SetAppraiser(r.Context(), a, team, pid, ids["userID"], in, true)
		case "create":
			var in CreateInput
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			out, err = h.svc.Create(r.Context(), a, team, pid, in)
			status = 201
		case "draft":
			var in DraftInput
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			out, err = h.svc.SaveDraft(r.Context(), a, team, pid, cid, in)
		case "share":
			var in ShareInput
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			out, err = h.svc.SetShare(r.Context(), a, team, pid, cid, ids["userID"], in, false)
		case "remove":
			var in VersionCommand
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			out, err = h.svc.SetShare(r.Context(), a, team, pid, cid, ids["userID"], ShareInput{VersionCommand: in}, true)
		case "submit", "withdraw", "abandon", "confirm", "revisions":
			var in VersionCommand
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			out, err = h.svc.VersionAction(r.Context(), a, team, pid, cid, action, in)
		case "dispute":
			var in DisputeInput
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			out, err = h.svc.RaiseDispute(r.Context(), a, team, pid, cid, in)
			status = 201
		case "resolve":
			var in ResolutionInput
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			out, err = h.svc.Resolve(r.Context(), a, team, pid, cid, ids["disputeID"], in)
		case "conflict":
			var in ConflictInput
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			out, err = h.svc.RecordConflict(r.Context(), a, team, pid, cid, in)
			status = 201
		case "notice":
			var in NoticeInput
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			out, err = h.svc.PrepareNotice(r.Context(), a, team, pid, cid, in)
			status = 201
		case "receipt":
			var in ReceiptInput
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			cookie, e := r.Cookie(auth.SessionCookieName)
			if e != nil {
				err = httpx.Unauthorized("authentication required")
				break
			}
			out, err = h.svc.Acknowledge(r.Context(), a, team, pid, cid, ids["noticeID"], in, cookie.Value)
		case "intake":
			var in IdentityRequestInput
			if !httpx.DecodeBody(w, r, h.logger, &in) {
				return
			}
			cookie, e := r.Cookie(auth.SessionCookieName)
			if e != nil {
				err = httpx.Unauthorized("authentication required")
				break
			}
			out, err = h.svc.ReceiveIdentityRequest(r.Context(), a, in, cookie.Value)
			status = 202
		default:
			err = httpx.NotFound("the requested resource does not exist")
		}
		if err != nil {
			httpx.WriteError(w, r, h.logger, err)
			return
		}
		httpx.Data(w, status, out)
	}
}
