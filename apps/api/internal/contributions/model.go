// Package contributions binds the approved database governance to authenticated
// business operations. PostgreSQL remains the authoritative state machine.
package contributions

import (
	"encoding/json"
	"github.com/google/uuid"
	"time"
)

type Command struct {
	ExpectedVersion int       `json:"expected_version"`
	ClientRequestID uuid.UUID `json:"client_request_id"`
	Reason          string    `json:"reason"`
}
type VersionCommand struct {
	Command
	CaseVersionID uuid.UUID `json:"case_version_id"`
}
type CreateInput struct {
	TaskID          uuid.UUID `json:"task_id"`
	ReviewID        uuid.UUID `json:"review_id"`
	ClientRequestID uuid.UUID `json:"client_request_id"`
	Reason          string    `json:"reason"`
}
type AppraiserInput struct {
	Command
	UserID uuid.UUID `json:"user_id"`
}
type EvidenceReference struct {
	Kind      string `json:"kind"`
	Source    string `json:"source"`
	Reference string `json:"reference"`
}
type ShareInput struct {
	VersionCommand
	ShareBPS     int                 `json:"share_bps"`
	Basis        string              `json:"basis"`
	Statement    string              `json:"statement"`
	EvidenceRefs []EvidenceReference `json:"evidence_refs"`
}
type DraftInput struct {
	VersionCommand
	Summary          string         `json:"summary"`
	EvidenceSnapshot map[string]any `json:"evidence_snapshot"`
}
type DisputeInput struct {
	VersionCommand
	ConfirmationID    uuid.UUID  `json:"confirmation_id"`
	OnBehalfOf        *uuid.UUID `json:"on_behalf_of,omitempty"`
	IdentityRequestID *uuid.UUID `json:"identity_request_id,omitempty"`
}
type ResolutionInput struct {
	Command
	Decision string `json:"decision"`
}
type ConflictInput struct {
	Command
	UserID  uuid.UUID `json:"user_id"`
	Kind    string    `json:"kind"`
	Details string    `json:"details"`
}
type NoticeInput struct {
	VersionCommand
	ConfirmationID uuid.UUID `json:"confirmation_id"`
	RecipientID    uuid.UUID `json:"recipient_id"`
}
type ReceiptInput struct {
	ExpectedVersion int       `json:"expected_version"`
	ClientRequestID uuid.UUID `json:"client_request_id"`
	CaseVersionID   uuid.UUID `json:"case_version_id"`
	ConfirmationID  uuid.UUID `json:"confirmation_id"`
	ContentDigest   string    `json:"content_digest"`
	Received        bool      `json:"received"`
}
type IdentityRequestInput struct {
	ClientRequestID uuid.UUID `json:"client_request_id"`
	TeamID          uuid.UUID `json:"team_id"`
	ProjectID       uuid.UUID `json:"project_id"`
	CaseID          uuid.UUID `json:"case_id"`
	CaseVersionID   uuid.UUID `json:"case_version_id"`
	ConfirmationID  uuid.UUID `json:"confirmation_id"`
	Reason          string    `json:"reason"`
}
type CommandResult struct {
	ID              uuid.UUID  `json:"id"`
	CaseID          *uuid.UUID `json:"case_id"`
	Revision        int        `json:"revision"`
	ClientRequestID uuid.UUID  `json:"client_request_id"`
}
type Intake struct {
	RequestID uuid.UUID `json:"request_id"`
	Received  bool      `json:"received"`
}
type Case struct {
	ID                          uuid.UUID  `json:"id"`
	TeamID                      uuid.UUID  `json:"team_id"`
	ProjectID                   uuid.UUID  `json:"project_id"`
	TaskID                      uuid.UUID  `json:"task_id"`
	SubmissionVersionID         uuid.UUID  `json:"submission_version_id"`
	ReviewID                    uuid.UUID  `json:"review_id"`
	LatestVersionID             uuid.UUID  `json:"latest_version_id"`
	EffectiveConfirmedVersionID *uuid.UUID `json:"effective_confirmed_version_id"`
	CaseWorkflowStatus          string     `json:"case_workflow_status"`
	Revision                    int        `json:"revision"`
	CreatedBy                   uuid.UUID  `json:"created_by"`
	CreatedAt                   time.Time  `json:"created_at"`
	UpdatedAt                   time.Time  `json:"updated_at"`
}
type Share struct {
	UserID          uuid.UUID           `json:"user_id"`
	ShareBPS        int                 `json:"share_bps"`
	Basis           string              `json:"basis"`
	Statement       string              `json:"statement"`
	EvidenceRefs    []EvidenceReference `json:"evidence_refs"`
	MembershipState string              `json:"membership_state"`
}
type Confirmation struct {
	ID                 uuid.UUID         `json:"id"`
	CaseVersionID      uuid.UUID         `json:"case_version_id"`
	ConfirmedBy        uuid.UUID         `json:"confirmed_by"`
	Rationale          string            `json:"rationale"`
	VersionDigest      string            `json:"version_digest"`
	MembershipSnapshot map[string]string `json:"membership_snapshot"`
	ConfirmedAt        time.Time         `json:"confirmed_at"`
}
type Version struct {
	ID                  uuid.UUID      `json:"id"`
	CaseID              uuid.UUID      `json:"case_id"`
	Version             int            `json:"version"`
	Status              string         `json:"status"`
	SupersedesVersionID *uuid.UUID     `json:"supersedes_version_id"`
	ProposedBy          uuid.UUID      `json:"proposed_by"`
	Summary             string         `json:"summary"`
	EvidenceSnapshot    map[string]any `json:"evidence_snapshot"`
	ProposedAt          time.Time      `json:"proposed_at"`
	SubmittedAt         *time.Time     `json:"submitted_at"`
	SubmittedDigest     *string        `json:"submitted_digest"`
	Shares              []Share        `json:"shares"`
	Confirmation        *Confirmation  `json:"confirmation"`
	TotalBPS            int            `json:"total_bps"`
	ReadyToSubmit       bool           `json:"ready_to_submit"`
}
type Permissions struct {
	CanEdit     bool `json:"can_edit"`
	CanSubmit   bool `json:"can_submit"`
	CanWithdraw bool `json:"can_withdraw"`
	CanConfirm  bool `json:"can_confirm"`
	CanRevise   bool `json:"can_revise"`
	CanDispute  bool `json:"can_dispute"`
	CanNotify   bool `json:"can_notify"`
}
type CaseDetail struct {
	Case        Case        `json:"case"`
	Latest      Version     `json:"latest"`
	Effective   *Version    `json:"effective"`
	Permissions Permissions `json:"permissions"`
}
type Eligibility struct {
	ExplicitAppraiser    bool `json:"explicit_appraiser"`
	CanEditCases         bool `json:"can_edit_cases"`
	AuthorizationVersion int  `json:"authorization_version"`
}
type Appraiser struct {
	UserID                     uuid.UUID `json:"user_id"`
	Status                     string    `json:"status"`
	Version                    int       `json:"version"`
	MemberAuthorizationVersion int       `json:"member_authorization_version"`
	Qualified                  bool      `json:"qualified"`
	GrantedBy                  uuid.UUID `json:"granted_by"`
	ChangedBy                  uuid.UUID `json:"changed_by"`
	Reason                     string    `json:"reason"`
	GrantedAt                  time.Time `json:"granted_at"`
}
type Resolution struct {
	ID         uuid.UUID `json:"id"`
	DisputeID  uuid.UUID `json:"dispute_id"`
	Decision   string    `json:"decision"`
	ResolvedBy uuid.UUID `json:"resolved_by"`
	Reason     string    `json:"reason"`
	ResolvedAt time.Time `json:"resolved_at"`
}
type Dispute struct {
	ID                uuid.UUID   `json:"id"`
	CaseVersionID     uuid.UUID   `json:"case_version_id"`
	ConfirmationID    uuid.UUID   `json:"confirmation_id"`
	RaisedBy          uuid.UUID   `json:"raised_by"`
	OnBehalfOf        *uuid.UUID  `json:"on_behalf_of"`
	IdentityRequestID *uuid.UUID  `json:"identity_request_id"`
	Reason            string      `json:"reason"`
	CreatedAt         time.Time   `json:"created_at"`
	Resolution        *Resolution `json:"resolution"`
	CanResolve        bool        `json:"can_resolve"`
}
type Event struct {
	ID            uuid.UUID      `json:"id"`
	CaseVersionID *uuid.UUID     `json:"case_version_id"`
	EventType     string         `json:"event_type"`
	ActorID       uuid.UUID      `json:"actor_id"`
	Reason        string         `json:"reason"`
	Details       map[string]any `json:"details"`
	RequestID     string         `json:"request_id"`
	CreatedAt     time.Time      `json:"created_at"`
}
type Adjustment struct {
	UserID        uuid.UUID       `json:"user_id"`
	CaseVersionID uuid.UUID       `json:"case_version_id"`
	OldBPS        *int            `json:"old_bps"`
	NewBPS        *int            `json:"new_bps"`
	BeforeValue   json.RawMessage `json:"before_value"`
	AfterValue    json.RawMessage `json:"after_value"`
	Reason        string          `json:"reason"`
	ActorID       uuid.UUID       `json:"actor_id"`
	RequestID     string          `json:"request_id"`
	CreatedAt     time.Time       `json:"created_at"`
}
type Conflict struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	Kind       string    `json:"kind"`
	Details    string    `json:"details"`
	RecordedBy uuid.UUID `json:"recorded_by"`
	CreatedAt  time.Time `json:"created_at"`
}

// Receipt deliberately has no session_id, token_hash or proof_token field.
type Receipt struct {
	ID             uuid.UUID `json:"id"`
	NoticeID       uuid.UUID `json:"notice_id"`
	CaseVersionID  uuid.UUID `json:"case_version_id"`
	ConfirmationID uuid.UUID `json:"confirmation_id"`
	SubjectID      uuid.UUID `json:"subject_id"`
	ContentDigest  string    `json:"content_digest"`
	VerifiedAt     time.Time `json:"verified_at"`
	SentAt         time.Time `json:"sent_at"`
	AcknowledgedAt time.Time `json:"acknowledged_at"`
}
type Notice struct {
	ID             uuid.UUID `json:"id"`
	CaseID         uuid.UUID `json:"case_id"`
	CaseVersionID  uuid.UUID `json:"case_version_id"`
	ConfirmationID uuid.UUID `json:"confirmation_id"`
	RecipientID    uuid.UUID `json:"recipient_id"`
	CreatedBy      uuid.UUID `json:"created_by"`
	Channel        string    `json:"channel"`
	PreparedAt     time.Time `json:"prepared_at"`
	Receipt        *Receipt  `json:"receipt"`
}
type Result struct {
	Notice       Notice       `json:"notice"`
	Confirmation Confirmation `json:"confirmation"`
	Summary      string       `json:"summary"`
	Shares       []Share      `json:"shares"`
}
type EvidenceAttachment struct {
	ID         uuid.UUID `json:"id"`
	UploaderID uuid.UUID `json:"uploader_id"`
	FileName   string    `json:"file_name"`
	ByteSize   int64     `json:"byte_size"`
	SHA256     string    `json:"sha256"`
}
type AcceptanceEvidence struct {
	TaskID              uuid.UUID            `json:"task_id"`
	TaskTitle           string               `json:"task_title"`
	TaskOwnerID         uuid.UUID            `json:"task_owner_id"`
	SubmissionVersionID uuid.UUID            `json:"submission_version_id"`
	AuthorID            uuid.UUID            `json:"author_id"`
	Summary             string               `json:"summary"`
	EvidenceReferences  json.RawMessage      `json:"evidence_references"`
	SubmittedAt         time.Time            `json:"submitted_at"`
	ReviewID            uuid.UUID            `json:"review_id"`
	ReviewerID          uuid.UUID            `json:"reviewer_id"`
	Decision            string               `json:"decision"`
	Comment             string               `json:"comment"`
	ReviewedAt          time.Time            `json:"reviewed_at"`
	Attachments         []EvidenceAttachment `json:"attachments"`
}
type History struct {
	Versions    []Version    `json:"versions"`
	Events      []Event      `json:"events"`
	Adjustments []Adjustment `json:"adjustments"`
	Disputes    []Dispute    `json:"disputes"`
	Conflicts   []Conflict   `json:"conflicts"`
	Notices     []Notice     `json:"notices"`
}
