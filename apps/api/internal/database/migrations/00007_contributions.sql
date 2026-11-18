-- +goose Up
-- Phase05 M2 only. No automatic cases, membership fabrication or archived backfill.
CREATE TABLE contribution_appraisers (
 team_id uuid NOT NULL, project_id uuid NOT NULL, user_id uuid NOT NULL,
 member_authorization_version integer NOT NULL CHECK(member_authorization_version>0),
 status text NOT NULL CHECK(status IN ('active','revoked')),
 granted_by uuid NOT NULL, reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 1000),
 version integer NOT NULL DEFAULT 1 CHECK(version>0),
 request_id text NOT NULL CHECK(length(btrim(request_id)) BETWEEN 1 AND 200),
 granted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(team_id,project_id,user_id), CHECK(user_id<>granted_by),
 FOREIGN KEY(team_id,project_id,user_id) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,granted_by) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT
);
CREATE TABLE contribution_cases (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), team_id uuid NOT NULL, project_id uuid NOT NULL, task_id uuid NOT NULL,
 submission_version_id uuid NOT NULL, review_id uuid NOT NULL UNIQUE,
 latest_version_id uuid, effective_confirmed_version_id uuid,
 case_workflow_status text NOT NULL DEFAULT 'draft' CHECK(case_workflow_status IN ('draft','pending_review','confirmed','disputed','revising','abandoned')),
 revision integer NOT NULL DEFAULT 1 CHECK(revision>0), created_by uuid NOT NULL,
 last_actor_id uuid NOT NULL, last_reason text NOT NULL CHECK(length(btrim(last_reason)) BETWEEN 1 AND 4000),
 request_id text NOT NULL CHECK(length(btrim(request_id)) BETWEEN 1 AND 200),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(team_id,project_id,task_id,id), UNIQUE(team_id,project_id,task_id,review_id),
 FOREIGN KEY(team_id,project_id,task_id) REFERENCES tasks(team_id,project_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,task_id,submission_version_id) REFERENCES task_submission_versions(team_id,project_id,task_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,task_id,submission_version_id,review_id) REFERENCES task_reviews(team_id,project_id,task_id,submission_version_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,created_by) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,last_actor_id) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT
);
CREATE TABLE contribution_case_versions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), team_id uuid NOT NULL, project_id uuid NOT NULL, task_id uuid NOT NULL, case_id uuid NOT NULL,
 version integer NOT NULL CHECK(version>0), status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','pending_review','confirmed','superseded','overturned','abandoned')),
 supersedes_version_id uuid, abandonment_resolution_id uuid,
 proposed_by uuid NOT NULL, summary text NOT NULL DEFAULT '' CHECK(length(summary)<=8000),
 evidence_snapshot jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(evidence_snapshot)='object'),
 client_request_id uuid NOT NULL DEFAULT gen_random_uuid(), submitted_at timestamptz, submitted_digest text CHECK(submitted_digest ~ '^[0-9a-f]{64}$'),
 changed_by uuid NOT NULL, change_reason text NOT NULL CHECK(length(btrim(change_reason)) BETWEEN 1 AND 4000),
 request_id text NOT NULL CHECK(length(btrim(request_id)) BETWEEN 1 AND 200),
 proposed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(team_id,project_id,task_id,case_id,id), UNIQUE(case_id,version), UNIQUE(case_id,client_request_id),
 FOREIGN KEY(team_id,project_id,task_id,case_id) REFERENCES contribution_cases(team_id,project_id,task_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,task_id,case_id,supersedes_version_id) REFERENCES contribution_case_versions(team_id,project_id,task_id,case_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,proposed_by) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,changed_by) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT,
 CHECK((status='draft' AND submitted_at IS NULL AND submitted_digest IS NULL) OR status='abandoned' OR (submitted_at IS NOT NULL AND submitted_digest IS NOT NULL))
);
CREATE UNIQUE INDEX contribution_one_working_version ON contribution_case_versions(team_id,project_id,task_id,case_id) WHERE status IN ('draft','pending_review');
ALTER TABLE contribution_cases ADD CONSTRAINT contribution_latest_fk FOREIGN KEY(team_id,project_id,task_id,id,latest_version_id) REFERENCES contribution_case_versions(team_id,project_id,task_id,case_id,id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE contribution_cases ADD CONSTRAINT contribution_effective_fk FOREIGN KEY(team_id,project_id,task_id,id,effective_confirmed_version_id) REFERENCES contribution_case_versions(team_id,project_id,task_id,case_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE contribution_shares (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), team_id uuid NOT NULL, project_id uuid NOT NULL, task_id uuid NOT NULL, case_id uuid NOT NULL, case_version_id uuid NOT NULL,
 user_id uuid NOT NULL, share_bps integer NOT NULL CHECK(share_bps BETWEEN 1 AND 10000),
 basis text NOT NULL DEFAULT '' CHECK(length(basis)<=4000), statement text NOT NULL DEFAULT '' CHECK(length(statement)<=8000),
 evidence_refs jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(evidence_refs)='array' AND jsonb_array_length(evidence_refs)<=50),
 membership_state text NOT NULL DEFAULT 'active' CHECK(membership_state IN ('active','historical')),
 changed_by uuid NOT NULL, change_reason text NOT NULL CHECK(length(btrim(change_reason)) BETWEEN 1 AND 4000),
 request_id text NOT NULL CHECK(length(btrim(request_id)) BETWEEN 1 AND 200),
 UNIQUE(case_version_id,user_id),
 FOREIGN KEY(team_id,project_id,task_id,case_id,case_version_id) REFERENCES contribution_case_versions(team_id,project_id,task_id,case_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,user_id) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,changed_by) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT
);
CREATE TABLE contribution_share_adjustments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), team_id uuid NOT NULL, project_id uuid NOT NULL, task_id uuid NOT NULL, case_id uuid NOT NULL, case_version_id uuid NOT NULL,
 user_id uuid NOT NULL, old_bps integer CHECK(old_bps BETWEEN 1 AND 10000), new_bps integer CHECK(new_bps BETWEEN 1 AND 10000),
 before_value jsonb, after_value jsonb, reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 4000), actor_id uuid NOT NULL,
 request_id text NOT NULL CHECK(length(btrim(request_id)) BETWEEN 1 AND 200), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK(old_bps IS NOT NULL OR new_bps IS NOT NULL),
 FOREIGN KEY(team_id,project_id,task_id,case_id,case_version_id) REFERENCES contribution_case_versions(team_id,project_id,task_id,case_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,user_id) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,actor_id) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT
);
CREATE TABLE contribution_confirmations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), team_id uuid NOT NULL, project_id uuid NOT NULL, task_id uuid NOT NULL, case_id uuid NOT NULL, case_version_id uuid NOT NULL UNIQUE,
 confirmed_by uuid NOT NULL, rationale text NOT NULL CHECK(length(btrim(rationale)) BETWEEN 1 AND 4000),
 version_digest text NOT NULL DEFAULT '' CHECK(version_digest ~ '^[0-9a-f]{64}$'), membership_snapshot jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(membership_snapshot)='object'),
 request_id text NOT NULL CHECK(length(btrim(request_id)) BETWEEN 1 AND 200), confirmed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(team_id,project_id,task_id,case_id,case_version_id,id),
 FOREIGN KEY(team_id,project_id,task_id,case_id,case_version_id) REFERENCES contribution_case_versions(team_id,project_id,task_id,case_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,confirmed_by) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT
);
CREATE TABLE contribution_conflicts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), team_id uuid NOT NULL, project_id uuid NOT NULL, task_id uuid NOT NULL, case_id uuid NOT NULL,
 user_id uuid NOT NULL, kind text NOT NULL CHECK(length(btrim(kind)) BETWEEN 1 AND 100), details text NOT NULL CHECK(length(btrim(details)) BETWEEN 1 AND 4000),
 recorded_by uuid NOT NULL, request_id text NOT NULL CHECK(length(btrim(request_id)) BETWEEN 1 AND 200), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(team_id,project_id,task_id,case_id) REFERENCES contribution_cases(team_id,project_id,task_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,user_id) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,recorded_by) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT
);
CREATE TABLE contribution_disputes (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), team_id uuid NOT NULL, project_id uuid NOT NULL, task_id uuid NOT NULL, case_id uuid NOT NULL, case_version_id uuid NOT NULL, confirmation_id uuid NOT NULL,
 raised_by uuid NOT NULL, on_behalf_of uuid, identity_request_id uuid,
 verification_method text NOT NULL DEFAULT '', verification_evidence_ref text NOT NULL DEFAULT '', verification_note text NOT NULL DEFAULT '' CHECK(length(verification_note)<=4000),
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 4000), request_id text NOT NULL CHECK(length(btrim(request_id)) BETWEEN 1 AND 200), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(team_id,project_id,task_id,case_id,id), UNIQUE(identity_request_id),
 FOREIGN KEY(team_id,project_id,task_id,case_id,case_version_id,confirmation_id) REFERENCES contribution_confirmations(team_id,project_id,task_id,case_id,case_version_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,raised_by) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,on_behalf_of) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT,
 CHECK((on_behalf_of IS NULL AND identity_request_id IS NULL) OR (on_behalf_of IS NOT NULL AND on_behalf_of<>raised_by AND identity_request_id IS NOT NULL AND verification_method='authenticated_request' AND length(btrim(verification_evidence_ref))>0))
);
CREATE TABLE contribution_dispute_resolutions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), team_id uuid NOT NULL, project_id uuid NOT NULL, task_id uuid NOT NULL, case_id uuid NOT NULL,
 dispute_id uuid NOT NULL UNIQUE, decision text NOT NULL CHECK(decision IN ('uphold','overturn')), resolved_by uuid NOT NULL,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 4000), request_id text NOT NULL CHECK(length(btrim(request_id)) BETWEEN 1 AND 200), resolved_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(team_id,project_id,task_id,case_id,id),
 FOREIGN KEY(team_id,project_id,task_id,case_id,dispute_id) REFERENCES contribution_disputes(team_id,project_id,task_id,case_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,resolved_by) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT
);
ALTER TABLE contribution_case_versions ADD CONSTRAINT contribution_retirement_ruling_fk FOREIGN KEY(team_id,project_id,task_id,case_id,abandonment_resolution_id) REFERENCES contribution_dispute_resolutions(team_id,project_id,task_id,case_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE contribution_case_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), team_id uuid NOT NULL, project_id uuid NOT NULL, task_id uuid NOT NULL, case_id uuid NOT NULL, case_version_id uuid,
 event_type text NOT NULL CHECK(length(btrim(event_type)) BETWEEN 1 AND 100), from_status text NOT NULL DEFAULT '', to_status text NOT NULL DEFAULT '',
 actor_id uuid NOT NULL, reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 4000), details jsonb NOT NULL DEFAULT '{}',
 request_id text NOT NULL CHECK(length(btrim(request_id)) BETWEEN 1 AND 200), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(team_id,project_id,task_id,case_id) REFERENCES contribution_cases(team_id,project_id,task_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,task_id,case_id,case_version_id) REFERENCES contribution_case_versions(team_id,project_id,task_id,case_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,actor_id) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT
);
CREATE TABLE contribution_notifications (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), team_id uuid NOT NULL, project_id uuid NOT NULL, task_id uuid NOT NULL, case_id uuid NOT NULL, case_version_id uuid NOT NULL, confirmation_id uuid NOT NULL,
 recipient_id uuid NOT NULL, channel text NOT NULL CHECK(channel IN ('authenticated_ack','manual_unverified')),
 created_by uuid NOT NULL, evidence_type text NOT NULL DEFAULT '', evidence_ref text NOT NULL DEFAULT '', evidence_digest text NOT NULL DEFAULT '', claimed_sent_at timestamptz,
 request_id text NOT NULL CHECK(length(btrim(request_id)) BETWEEN 1 AND 200), prepared_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(team_id,project_id,task_id,case_id,case_version_id,confirmation_id,id),
 FOREIGN KEY(team_id,project_id,task_id,case_id,case_version_id,confirmation_id) REFERENCES contribution_confirmations(team_id,project_id,task_id,case_id,case_version_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,recipient_id) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,created_by) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT
);
CREATE TABLE contribution_attestations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), team_id uuid NOT NULL, project_id uuid NOT NULL, task_id uuid NOT NULL, case_id uuid NOT NULL, case_version_id uuid NOT NULL, confirmation_id uuid NOT NULL,
 purpose text NOT NULL CHECK(purpose IN ('delivery','dispute_request')), notice_id uuid,
 subject_id uuid NOT NULL, actor_id uuid NOT NULL, session_id uuid NOT NULL, proof_token text,
 statement text NOT NULL CHECK(length(btrim(statement)) BETWEEN 1 AND 4000), subject_digest text NOT NULL CHECK(subject_digest ~ '^[0-9a-f]{64}$'),
 channel text NOT NULL DEFAULT 'authenticated_ack' CHECK(channel='authenticated_ack'),
 sent_at timestamptz NOT NULL DEFAULT clock_timestamp(), acknowledged_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 request_id text NOT NULL CHECK(length(btrim(request_id)) BETWEEN 1 AND 200),
 UNIQUE(team_id,project_id,task_id,case_id,case_version_id,id), UNIQUE(notice_id),
 CHECK(proof_token IS NULL), CHECK(actor_id=subject_id), CHECK((purpose='delivery' AND notice_id IS NOT NULL) OR (purpose='dispute_request' AND notice_id IS NULL)),
 FOREIGN KEY(team_id,project_id,task_id,case_id,case_version_id,confirmation_id) REFERENCES contribution_confirmations(team_id,project_id,task_id,case_id,case_version_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,task_id,case_id,case_version_id,confirmation_id,notice_id) REFERENCES contribution_notifications(team_id,project_id,task_id,case_id,case_version_id,confirmation_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,subject_id) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT,
 FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE RESTRICT
);
ALTER TABLE contribution_disputes ADD CONSTRAINT contribution_identity_request_fk FOREIGN KEY(team_id,project_id,task_id,case_id,case_version_id,identity_request_id) REFERENCES contribution_attestations(team_id,project_id,task_id,case_id,case_version_id,id) ON DELETE RESTRICT;
CREATE INDEX contribution_cases_project_status_idx ON contribution_cases(team_id,project_id,case_workflow_status,id);
CREATE INDEX contribution_versions_history_idx ON contribution_case_versions(team_id,project_id,task_id,case_id,version DESC);
CREATE INDEX contribution_shares_user_idx ON contribution_shares(team_id,project_id,user_id,case_version_id);
CREATE INDEX contribution_disputes_case_idx ON contribution_disputes(team_id,project_id,task_id,case_id,created_at);
CREATE INDEX contribution_events_case_idx ON contribution_case_events(team_id,project_id,task_id,case_id,created_at,id);
CREATE INDEX contribution_adjustments_version_idx ON contribution_share_adjustments(team_id,project_id,task_id,case_id,case_version_id,created_at);
CREATE INDEX contribution_conflicts_case_idx ON contribution_conflicts(team_id,project_id,task_id,case_id,user_id);
CREATE INDEX contribution_notices_recipient_idx ON contribution_notifications(team_id,project_id,recipient_id,confirmation_id);
CREATE INDEX contribution_receipts_version_idx ON contribution_attestations(team_id,project_id,task_id,case_id,case_version_id,purpose,subject_id);

-- P05_INTEGRITY

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM contribution_cases) OR EXISTS(SELECT 1 FROM contribution_appraisers) THEN RAISE EXCEPTION 'cannot roll back contribution business or governance evidence'; END IF;
END $$;
-- +goose StatementEnd
-- P05_DOWN_FUNCTIONS
ALTER TABLE contribution_disputes DROP CONSTRAINT contribution_identity_request_fk;
ALTER TABLE contribution_case_versions DROP CONSTRAINT contribution_retirement_ruling_fk;
ALTER TABLE contribution_cases DROP CONSTRAINT contribution_latest_fk;
ALTER TABLE contribution_cases DROP CONSTRAINT contribution_effective_fk;
DROP TABLE contribution_attestations,contribution_notifications,contribution_case_events,contribution_dispute_resolutions,contribution_disputes,contribution_conflicts,contribution_confirmations,contribution_share_adjustments,contribution_shares,contribution_case_versions,contribution_cases,contribution_appraisers;
