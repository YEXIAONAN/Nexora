// Package submissions owns immutable delivery versions, explicit review grants
// and private attachments. It delegates every task state write to tasks.
package submissions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/YEXIAONAN/Nexora/apps/api/internal/httpx"
	"github.com/google/uuid"
)

type Reference struct {
	Kind  string `json:"kind"`
	URL   string `json:"url"`
	Label string `json:"label"`
}
type SubmitInput struct {
	ClientRequestID uuid.UUID   `json:"client_request_id"`
	ExpectedVersion int         `json:"expected_version"`
	Summary         string      `json:"summary"`
	References      []Reference `json:"references"`
	AttachmentIDs   []uuid.UUID `json:"attachment_ids"`
}

const MaxVersionBytes int64 = 25 << 20

type ReviewInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Decision        string `json:"decision"`
	Comment         string `json:"comment"`
}
type Version struct {
	ID                 uuid.UUID    `json:"id"`
	TeamID             uuid.UUID    `json:"team_id"`
	ProjectID          uuid.UUID    `json:"project_id"`
	TaskID             uuid.UUID    `json:"task_id"`
	SubmissionID       uuid.UUID    `json:"submission_id"`
	Version            int          `json:"version"`
	AuthorID           uuid.UUID    `json:"author_id"`
	AuthorName         string       `json:"author_name"`
	Summary            string       `json:"summary"`
	References         []Reference  `json:"references"`
	Attachments        []Attachment `json:"attachments"`
	SubmittedAt        time.Time    `json:"submitted_at"`
	BasedOnTaskVersion int          `json:"based_on_task_version"`
	Review             *Review      `json:"review"`
}
type Review struct {
	ID                  uuid.UUID `json:"id"`
	TeamID              uuid.UUID `json:"team_id"`
	ProjectID           uuid.UUID `json:"project_id"`
	TaskID              uuid.UUID `json:"task_id"`
	SubmissionVersionID uuid.UUID `json:"submission_version_id"`
	ReviewerID          uuid.UUID `json:"reviewer_id"`
	ReviewerName        string    `json:"reviewer_name"`
	Decision            string    `json:"decision"`
	Comment             string    `json:"comment"`
	ReviewedAt          time.Time `json:"reviewed_at"`
	RequestID           string    `json:"request_id"`
	TaskVersion         int       `json:"task_version"`
}
type Attachment struct {
	ID                  uuid.UUID  `json:"id"`
	TeamID              uuid.UUID  `json:"team_id"`
	ProjectID           uuid.UUID  `json:"project_id"`
	TaskID              uuid.UUID  `json:"task_id"`
	UploaderID          uuid.UUID  `json:"uploader_id"`
	SubmissionVersionID *uuid.UUID `json:"submission_version_id"`
	FileName            string     `json:"file_name"`
	MediaType           string     `json:"media_type"`
	ByteSize            int64      `json:"byte_size"`
	SHA256              string     `json:"sha256"`
	Status              string     `json:"status"`
	CreatedAt           time.Time  `json:"created_at"`
	ExpiresAt           time.Time  `json:"expires_at"`
	RemovedAt           *time.Time `json:"removed_at"`
	storageKey          string
}
type Reviewer struct {
	UserID      uuid.UUID  `json:"user_id"`
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	DefaultLead bool       `json:"default_lead"`
	Active      bool       `json:"active"`
	Version     int        `json:"version"`
	GrantedBy   *uuid.UUID `json:"granted_by"`
	GrantedAt   *time.Time `json:"granted_at"`
	Reason      string     `json:"reason"`
}
type Pending struct {
	ProjectID           uuid.UUID `json:"project_id"`
	ProjectName         string    `json:"project_name"`
	TaskID              uuid.UUID `json:"task_id"`
	TaskTitle           string    `json:"task_title"`
	TaskVersion         int       `json:"task_version"`
	SubmissionVersionID uuid.UUID `json:"submission_version_id"`
	Version             int       `json:"version"`
	AuthorName          string    `json:"author_name"`
	SubmittedAt         time.Time `json:"submitted_at"`
}

func normalize(in *SubmitInput) (string, error) {
	in.Summary = strings.TrimSpace(in.Summary)
	fields := map[string]string{}
	if in.ClientRequestID == uuid.Nil {
		fields["client_request_id"] = "a nonzero UUID is required"
	}
	if n := utf8.RuneCountInString(in.Summary); n > 8000 || !utf8.ValidString(in.Summary) || strings.ContainsRune(in.Summary, 0) {
		fields["summary"] = "must be valid text up to 8000 characters"
	}
	if in.References == nil {
		in.References = []Reference{}
	}
	if in.AttachmentIDs == nil {
		in.AttachmentIDs = []uuid.UUID{}
	}
	if len(in.References) > 20 {
		fields["references"] = "at most 20 references"
	}
	for i := range in.References {
		r := &in.References[i]
		r.URL = strings.TrimSpace(r.URL)
		r.Label = strings.TrimSpace(r.Label)
		u, e := url.ParseRequestURI(r.URL)
		if e != nil || u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || len(r.URL) > 2048 || strings.ContainsFunc(r.URL, func(c rune) bool { return unicode.IsSpace(c) || unicode.IsControl(c) }) || utf8.RuneCountInString(r.Label) > 200 || strings.ContainsRune(r.Label, 0) || (r.Kind != "link" && r.Kind != "git_commit" && r.Kind != "pull_request") {
			fields["references"] = "valid HTTP(S) evidence URLs, kinds and labels are required"
		}
	}
	if len(in.AttachmentIDs) > 5 {
		fields["attachment_ids"] = "at most 5 files per version"
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range in.AttachmentIDs {
		if id == uuid.Nil || seen[id] {
			fields["attachment_ids"] = "must contain unique nonzero UUIDs"
		}
		seen[id] = true
	}
	if in.Summary == "" && len(in.References) == 0 && len(in.AttachmentIDs) == 0 {
		fields["summary"] = "provide a description, reference or attachment"
	}
	if len(fields) > 0 {
		return "", httpx.ValidationFailed(fields)
	}
	// File order has no business meaning; reordering a retry remains idempotent.
	sort.Slice(in.AttachmentIDs, func(i, j int) bool { return in.AttachmentIDs[i].String() < in.AttachmentIDs[j].String() })
	b, e := json.Marshal(struct {
		Summary     string
		References  []Reference
		Attachments []uuid.UUID
	}{in.Summary, in.References, in.AttachmentIDs})
	if e != nil {
		return "", e
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
