-- +goose Up
-- Phase05 M2 only. No automatic cases, membership fabrication or archived backfill.
CREATE TABLE contribution_appraisers (
 team_id uuid NOT NULL, project_id uuid NOT NULL, user_id uuid NOT NULL,
 member_authorization_version integer NOT NULL CHECK(member_authorization_version>0),
 status text NOT NULL CHECK(status IN ('active','revoked')),
 granted_by uuid NOT NULL, changed_by uuid NOT NULL, reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 1000),
 version integer NOT NULL DEFAULT 1 CHECK(version>0),
 request_id text NOT NULL CHECK(length(btrim(request_id)) BETWEEN 1 AND 200),
 granted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(team_id,project_id,user_id), CHECK(user_id<>granted_by),
 FOREIGN KEY(team_id,project_id,user_id) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,granted_by) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,changed_by) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT
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
 statement text NOT NULL DEFAULT '' CHECK(length(statement)<=4000), content_digest text NOT NULL CHECK(content_digest ~ '^[0-9a-f]{64}$'),
 channel text NOT NULL DEFAULT 'authenticated_ack' CHECK(channel='authenticated_ack'),
 verified_at timestamptz NOT NULL DEFAULT clock_timestamp(), sent_at timestamptz, acknowledged_at timestamptz,
 request_id text NOT NULL CHECK(length(btrim(request_id)) BETWEEN 1 AND 200),
 UNIQUE(team_id,project_id,task_id,case_id,case_version_id,id), UNIQUE(notice_id),
 CHECK(proof_token IS NULL), CHECK(actor_id=subject_id), CHECK((purpose='delivery' AND notice_id IS NOT NULL AND sent_at IS NOT NULL AND acknowledged_at IS NOT NULL) OR (purpose='dispute_request' AND notice_id IS NULL AND sent_at IS NULL AND acknowledged_at IS NULL)),
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

-- +goose StatementBegin
CREATE FUNCTION contribution_member(p_team uuid,p_project uuid,p_user uuid) RETURNS boolean AS $$
 SELECT EXISTS(SELECT 1 FROM project_members pm JOIN team_members tm ON tm.team_id=pm.team_id AND tm.user_id=pm.user_id WHERE pm.team_id=p_team AND pm.project_id=p_project AND pm.user_id=p_user AND pm.status='active' AND tm.status='active');
$$ LANGUAGE sql STABLE;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_appraiser(p_team uuid,p_project uuid,p_user uuid) RETURNS boolean AS $$
 SELECT contribution_member(p_team,p_project,p_user) AND EXISTS(SELECT 1 FROM contribution_appraisers a JOIN project_members pm ON pm.team_id=a.team_id AND pm.project_id=a.project_id AND pm.user_id=a.user_id WHERE a.team_id=p_team AND a.project_id=p_project AND a.user_id=p_user AND a.status='active' AND a.member_authorization_version=pm.authorization_version);
$$ LANGUAGE sql STABLE;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_editor(p_team uuid,p_project uuid,p_user uuid) RETURNS boolean AS $$
 SELECT contribution_member(p_team,p_project,p_user) AND (EXISTS(SELECT 1 FROM projects WHERE team_id=p_team AND id=p_project AND lead_id=p_user) OR contribution_appraiser(p_team,p_project,p_user));
$$ LANGUAGE sql STABLE;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_lock(p_team uuid,p_project uuid,p_case uuid) RETURNS void AS $$
BEGIN
 IF current_setting('transaction_isolation')<>'read committed' THEN RAISE EXCEPTION 'contribution writes require read committed snapshots after the team lock' USING ERRCODE='25001'; END IF;
 PERFORM 1 FROM teams WHERE id=p_team FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'unknown team' USING ERRCODE='23503'; END IF;
 IF NOT EXISTS(SELECT 1 FROM projects WHERE team_id=p_team AND id=p_project AND status='active') THEN RAISE EXCEPTION 'archived or unknown project' USING ERRCODE='23514',CONSTRAINT='contribution_project_readonly'; END IF;
 IF p_case IS NOT NULL THEN
  PERFORM 1 FROM contribution_cases WHERE team_id=p_team AND project_id=p_project AND id=p_case FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION 'unknown scoped contribution case' USING ERRCODE='23503'; END IF;
 END IF;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_expected(p_team uuid,p_project uuid,p_case uuid,p_expected integer) RETURNS void AS $$
DECLARE n integer;
BEGIN
 PERFORM contribution_lock(p_team,p_project,p_case);
 SELECT revision INTO n FROM contribution_cases WHERE team_id=p_team AND project_id=p_project AND id=p_case;
 IF n IS DISTINCT FROM p_expected THEN RAISE EXCEPTION 'contribution expected_version conflict' USING ERRCODE='40001'; END IF;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_emit(p_team uuid,p_project uuid,p_task uuid,p_case uuid,p_version uuid,p_event text,p_actor uuid,p_reason text,p_request text,p_details jsonb) RETURNS void AS $$
BEGIN
 INSERT INTO contribution_case_events(team_id,project_id,task_id,case_id,case_version_id,event_type,actor_id,reason,request_id,details) VALUES(p_team,p_project,p_task,p_case,p_version,p_event,p_actor,p_reason,p_request,COALESCE(p_details,'{}'));
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_event_audit() RETURNS trigger AS $$
BEGIN
 INSERT INTO audit_events(actor_id,action,target_type,target_id,team_id,request_id,details) VALUES(NEW.actor_id,'contribution.'||NEW.event_type,'contribution_case',NEW.case_id,NEW.team_id,NEW.request_id,jsonb_build_object('project_id',NEW.project_id,'task_id',NEW.task_id,'case_version_id',NEW.case_version_id,'event_id',NEW.id,'reason',NEW.reason,'details',NEW.details));
 RETURN NULL;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_events_audit AFTER INSERT ON contribution_case_events FOR EACH ROW EXECUTE FUNCTION contribution_event_audit();
-- +goose StatementBegin
CREATE FUNCTION contribution_touch(p_case uuid,p_actor uuid,p_reason text,p_request text) RETURNS void AS $$
BEGIN
 UPDATE contribution_cases SET revision=revision+1,last_actor_id=p_actor,last_reason=p_reason,request_id=p_request WHERE id=p_case;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_open_disputes(p_case uuid) RETURNS boolean AS $$
 SELECT EXISTS(SELECT 1 FROM contribution_disputes d WHERE d.case_id=p_case AND NOT EXISTS(SELECT 1 FROM contribution_dispute_resolutions r WHERE r.dispute_id=d.id));
$$ LANGUAGE sql STABLE;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_digest(p_version uuid) RETURNS text AS $$
 SELECT encode(sha256(convert_to(jsonb_build_object('version',v.id,'case',v.case_id,'number',v.version,'proposer',v.proposed_by,'summary',v.summary,'evidence',v.evidence_snapshot,'shares',COALESCE((SELECT jsonb_agg(jsonb_build_object('user',s.user_id,'bps',s.share_bps,'basis',s.basis,'statement',s.statement,'evidence',s.evidence_refs) ORDER BY s.user_id) FROM contribution_shares s WHERE s.case_version_id=v.id),'[]'::jsonb))::text,'UTF8')),'hex') FROM contribution_case_versions v WHERE v.id=p_version;
$$ LANGUAGE sql STABLE;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_complete_shares(p_version uuid) RETURNS boolean AS $$
 SELECT COALESCE(sum(s.share_bps),0)=10000 AND count(*)>0 AND COALESCE(bool_and(length(btrim(s.basis))>0 AND jsonb_array_length(s.evidence_refs)>0 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(s.evidence_refs) e WHERE jsonb_typeof(e)<>'object' OR COALESCE(e->>'kind','') NOT IN ('objective_fact','self_statement','third_party') OR length(btrim(COALESCE(e->>'source','')))=0 OR length(btrim(COALESCE(e->>'reference','')))=0)),false) FROM contribution_shares s WHERE s.case_version_id=p_version;
$$ LANGUAGE sql STABLE;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_independent(p_version uuid,p_actor uuid) RETURNS boolean AS $$
 SELECT contribution_appraiser(v.team_id,v.project_id,p_actor)
 AND v.proposed_by<>p_actor AND sv.author_id<>p_actor AND tr.reviewer_id<>p_actor
 AND NOT EXISTS(SELECT 1 FROM contribution_shares s WHERE s.case_version_id=v.id AND s.user_id=p_actor)
 AND NOT EXISTS(SELECT 1 FROM contribution_conflicts f WHERE f.case_id=v.case_id AND f.user_id=p_actor)
 FROM contribution_case_versions v JOIN contribution_cases c ON c.id=v.case_id JOIN task_submission_versions sv ON sv.id=c.submission_version_id JOIN task_reviews tr ON tr.id=c.review_id WHERE v.id=p_version;
$$ LANGUAGE sql STABLE;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_case_guard() RETURNS trigger AS $$
DECLARE latest uuid; effective uuid; state text; latest_state text;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'contribution cases are historical evidence' USING ERRCODE='23514'; END IF;
 PERFORM contribution_lock(NEW.team_id,NEW.project_id,CASE WHEN TG_OP='UPDATE' THEN NEW.id ELSE NULL END);
 IF TG_OP='INSERT' THEN
  IF NOT contribution_editor(NEW.team_id,NEW.project_id,NEW.created_by) OR NEW.last_actor_id<>NEW.created_by OR NEW.revision<>1 OR NEW.latest_version_id IS NOT NULL OR NEW.effective_confirmed_version_id IS NOT NULL OR NEW.case_workflow_status<>'draft' THEN RAISE EXCEPTION 'invalid case creation or actor' USING ERRCODE='23514'; END IF;
 ELSE
  IF pg_trigger_depth()<2 THEN RAISE EXCEPTION 'case pointers are maintained by contribution triggers' USING ERRCODE='23514'; END IF;
  IF (to_jsonb(NEW)-ARRAY['latest_version_id','effective_confirmed_version_id','case_workflow_status','revision','last_actor_id','last_reason','request_id','updated_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['latest_version_id','effective_confirmed_version_id','case_workflow_status','revision','last_actor_id','last_reason','request_id','updated_at']) OR NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION 'case identity and revision are controlled' USING ERRCODE='23514'; END IF;
  SELECT id,status INTO latest,latest_state FROM contribution_case_versions WHERE case_id=NEW.id ORDER BY version DESC LIMIT 1;
  SELECT v.id INTO effective FROM contribution_case_versions v JOIN contribution_confirmations f ON f.case_version_id=v.id WHERE v.case_id=NEW.id AND v.status='confirmed' ORDER BY v.version DESC LIMIT 1;
  state:=CASE WHEN contribution_open_disputes(NEW.id) THEN 'disputed' WHEN effective IS NOT NULL AND latest<>effective AND latest_state IN ('draft','pending_review') THEN 'revising' WHEN effective IS NOT NULL THEN 'confirmed' WHEN latest_state='pending_review' THEN 'pending_review' WHEN latest_state='abandoned' THEN 'abandoned' ELSE 'draft' END;
  IF (NEW.latest_version_id IS DISTINCT FROM OLD.latest_version_id AND NEW.latest_version_id IS DISTINCT FROM latest) OR (NEW.effective_confirmed_version_id IS DISTINCT FROM OLD.effective_confirmed_version_id AND NEW.effective_confirmed_version_id IS DISTINCT FROM effective) OR (NEW.case_workflow_status<>OLD.case_workflow_status AND NEW.case_workflow_status<>state) THEN RAISE EXCEPTION 'case pointers and workflow are derived' USING ERRCODE='23514'; END IF;
  NEW.latest_version_id:=latest;NEW.effective_confirmed_version_id:=effective;NEW.case_workflow_status:=state;NEW.updated_at:=clock_timestamp();
 END IF;
 IF NOT EXISTS(SELECT 1 FROM tasks t JOIN task_reviews r ON r.team_id=t.team_id AND r.project_id=t.project_id AND r.task_id=t.id AND r.id=t.accepted_review_id AND r.submission_version_id=t.current_submission_version_id WHERE t.team_id=NEW.team_id AND t.project_id=NEW.project_id AND t.id=NEW.task_id AND t.status='completed' AND t.current_submission_version_id=NEW.submission_version_id AND t.accepted_review_id=NEW.review_id AND r.decision='approve') THEN RAISE EXCEPTION 'a current real task approval is required' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_cases_guard BEFORE INSERT OR UPDATE OR DELETE ON contribution_cases FOR EACH ROW EXECUTE FUNCTION contribution_case_guard();
-- +goose StatementBegin
CREATE FUNCTION contribution_case_created() RETURNS trigger AS $$
BEGIN
 INSERT INTO contribution_case_versions(team_id,project_id,task_id,case_id,version,proposed_by,changed_by,change_reason,request_id) VALUES(NEW.team_id,NEW.project_id,NEW.task_id,NEW.id,1,NEW.created_by,NEW.created_by,NEW.last_reason,NEW.request_id);
 PERFORM contribution_emit(NEW.team_id,NEW.project_id,NEW.task_id,NEW.id,NULL,'case_created',NEW.created_by,NEW.last_reason,NEW.request_id,jsonb_build_object('review_id',NEW.review_id)); RETURN NULL;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_cases_created AFTER INSERT ON contribution_cases FOR EACH ROW EXECUTE FUNCTION contribution_case_created();
-- +goose StatementBegin
CREATE FUNCTION contribution_version_guard() RETURNS trigger AS $$
DECLARE c contribution_cases%ROWTYPE; n integer; internal_retirement boolean;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'version history cannot be deleted' USING ERRCODE='23514'; END IF;
 PERFORM contribution_lock(NEW.team_id,NEW.project_id,NEW.case_id);
 SELECT * INTO c FROM contribution_cases WHERE id=NEW.case_id;
 IF TG_OP='INSERT' THEN
  SELECT COALESCE(max(version),0)+1 INTO n FROM contribution_case_versions WHERE case_id=NEW.case_id;
  IF NEW.status<>'draft' OR NEW.version<>n OR NOT contribution_editor(NEW.team_id,NEW.project_id,NEW.proposed_by) OR NEW.changed_by<>NEW.proposed_by OR contribution_open_disputes(NEW.case_id) AND pg_trigger_depth()<2 THEN RAISE EXCEPTION 'invalid draft or proposer' USING ERRCODE='23514'; END IF;
  IF n=1 AND NEW.supersedes_version_id IS NOT NULL OR n>1 AND NOT EXISTS(SELECT 1 FROM contribution_case_versions WHERE case_id=NEW.case_id AND id=NEW.supersedes_version_id AND status IN ('confirmed','overturned','abandoned','superseded')) THEN RAISE EXCEPTION 'revision requires real predecessor evidence' USING ERRCODE='23514'; END IF;
 ELSE
  IF (to_jsonb(NEW)-ARRAY['status','summary','evidence_snapshot','submitted_at','submitted_digest','changed_by','change_reason','request_id','abandonment_resolution_id']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['status','summary','evidence_snapshot','submitted_at','submitted_digest','changed_by','change_reason','request_id','abandonment_resolution_id']) THEN RAISE EXCEPTION 'version identity is immutable' USING ERRCODE='23514'; END IF;
  IF (OLD.status<>'draft' OR NEW.status<>'draft') AND (NEW.summary<>OLD.summary OR NEW.evidence_snapshot<>OLD.evidence_snapshot) THEN RAISE EXCEPTION 'submitted version content is frozen' USING ERRCODE='23514'; END IF;
  IF OLD.status IN ('superseded','overturned','abandoned') OR OLD.status='confirmed' AND NEW.status=OLD.status OR OLD.status='pending_review' AND NEW.status=OLD.status THEN RAISE EXCEPTION 'frozen version cannot be edited' USING ERRCODE='23514'; END IF;
  internal_retirement:=pg_trigger_depth()>1 AND NEW.abandonment_resolution_id IS NOT NULL AND EXISTS(SELECT 1 FROM contribution_dispute_resolutions r WHERE r.id=NEW.abandonment_resolution_id AND r.case_id=NEW.case_id AND r.decision='overturn' AND r.resolved_by=NEW.changed_by);
  IF NEW.status<>OLD.status THEN
   IF NOT ((OLD.status='draft' AND NEW.status IN ('pending_review','abandoned')) OR (OLD.status='pending_review' AND NEW.status IN ('draft','confirmed')) OR (OLD.status='confirmed' AND NEW.status IN ('superseded','overturned')) OR (OLD.status='pending_review' AND NEW.status='abandoned' AND internal_retirement)) THEN RAISE EXCEPTION 'illegal contribution version transition' USING ERRCODE='23514'; END IF;
   IF NEW.status='confirmed' THEN
    IF NOT EXISTS(SELECT 1 FROM contribution_confirmations f WHERE f.case_version_id=NEW.id AND f.confirmed_by=NEW.changed_by) THEN RAISE EXCEPTION 'formal confirmation is required' USING ERRCODE='23514'; END IF;
   ELSIF NEW.status='superseded' THEN
    IF NOT EXISTS(SELECT 1 FROM contribution_case_versions v JOIN contribution_confirmations f ON f.case_version_id=v.id WHERE v.case_id=NEW.case_id AND v.version>NEW.version AND v.status='confirmed') THEN RAISE EXCEPTION 'newer formal confirmation is required' USING ERRCODE='23514'; END IF;
   ELSIF NEW.status='overturned' THEN
    IF NOT EXISTS(SELECT 1 FROM contribution_disputes d JOIN contribution_dispute_resolutions r ON r.dispute_id=d.id WHERE d.case_version_id=NEW.id AND r.decision='overturn' AND r.resolved_by=NEW.changed_by) THEN RAISE EXCEPTION 'formal overturn is required' USING ERRCODE='23514'; END IF;
   ELSE
    IF NOT internal_retirement AND (NOT contribution_editor(NEW.team_id,NEW.project_id,NEW.changed_by) OR EXISTS(SELECT 1 FROM contribution_shares WHERE case_version_id=NEW.id AND user_id=NEW.changed_by)) THEN RAISE EXCEPTION 'editor is unqualified or a beneficiary' USING ERRCODE='23514'; END IF;
    IF NEW.status='draft' AND NEW.changed_by<>OLD.proposed_by THEN RAISE EXCEPTION 'only proposer may withdraw' USING ERRCODE='23514'; END IF;
   END IF;
  ELSIF NOT contribution_editor(NEW.team_id,NEW.project_id,NEW.changed_by) OR EXISTS(SELECT 1 FROM contribution_shares WHERE case_version_id=NEW.id AND user_id=NEW.changed_by) THEN RAISE EXCEPTION 'unqualified draft editor' USING ERRCODE='23514'; END IF;
 END IF;
 IF NEW.status='pending_review' THEN
  IF contribution_open_disputes(NEW.case_id) OR NOT contribution_complete_shares(NEW.id) THEN RAISE EXCEPTION 'submission requires complete 10000 bps and no open dispute' USING ERRCODE='23514'; END IF;
  NEW.submitted_at:=clock_timestamp();NEW.submitted_digest:=contribution_digest(NEW.id);
 ELSIF NEW.status='draft' THEN NEW.submitted_at:=NULL;NEW.submitted_digest:=NULL;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_versions_guard BEFORE INSERT OR UPDATE OR DELETE ON contribution_case_versions FOR EACH ROW EXECUTE FUNCTION contribution_version_guard();
-- +goose StatementBegin
CREATE FUNCTION contribution_version_event() RETURNS trigger AS $$
BEGIN
 PERFORM contribution_touch(NEW.case_id,NEW.changed_by,NEW.change_reason,NEW.request_id);
 PERFORM contribution_emit(NEW.team_id,NEW.project_id,NEW.task_id,NEW.case_id,NEW.id,CASE WHEN TG_OP='INSERT' THEN 'version_created' ELSE 'version_'||NEW.status END,NEW.changed_by,NEW.change_reason,NEW.request_id,jsonb_build_object('status',NEW.status,'version',NEW.version)); RETURN NULL;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_versions_events AFTER INSERT OR UPDATE ON contribution_case_versions FOR EACH ROW EXECUTE FUNCTION contribution_version_event();
-- +goose StatementBegin
CREATE FUNCTION contribution_share_guard() RETURNS trigger AS $$
DECLARE j jsonb; v contribution_case_versions%ROWTYPE; actor uuid;
BEGIN
 j:=CASE WHEN TG_OP='DELETE' THEN to_jsonb(OLD) ELSE to_jsonb(NEW) END;
 PERFORM contribution_lock((j->>'team_id')::uuid,(j->>'project_id')::uuid,(j->>'case_id')::uuid);
 SELECT * INTO v FROM contribution_case_versions WHERE id=(j->>'case_version_id')::uuid AND team_id=(j->>'team_id')::uuid AND project_id=(j->>'project_id')::uuid AND task_id=(j->>'task_id')::uuid AND case_id=(j->>'case_id')::uuid;
 actor:=(j->>'changed_by')::uuid;
 IF v.status IS DISTINCT FROM 'draft' OR NOT contribution_editor(v.team_id,v.project_id,actor) OR actor=(j->>'user_id')::uuid OR EXISTS(SELECT 1 FROM contribution_shares WHERE case_version_id=v.id AND user_id=actor) THEN RAISE EXCEPTION 'shares require a draft and non-beneficiary editor' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' AND (NEW.id<>OLD.id OR NEW.team_id<>OLD.team_id OR NEW.project_id<>OLD.project_id OR NEW.task_id<>OLD.task_id OR NEW.case_id<>OLD.case_id OR NEW.case_version_id<>OLD.case_version_id OR NEW.user_id<>OLD.user_id) THEN RAISE EXCEPTION 'share identity is immutable' USING ERRCODE='23514'; END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 NEW.membership_state:=CASE WHEN contribution_member(NEW.team_id,NEW.project_id,NEW.user_id) THEN 'active' ELSE 'historical' END; RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_shares_guard BEFORE INSERT OR UPDATE OR DELETE ON contribution_shares FOR EACH ROW EXECUTE FUNCTION contribution_share_guard();
-- +goose StatementBegin
CREATE FUNCTION contribution_share_event() RETURNS trigger AS $$
DECLARE j jsonb; before_j jsonb; after_j jsonb;
BEGIN
 j:=CASE WHEN TG_OP='DELETE' THEN to_jsonb(OLD) ELSE to_jsonb(NEW) END;
 IF TG_OP<>'INSERT' THEN before_j:=to_jsonb(OLD); END IF;IF TG_OP<>'DELETE' THEN after_j:=to_jsonb(NEW); END IF;
 INSERT INTO contribution_share_adjustments(team_id,project_id,task_id,case_id,case_version_id,user_id,old_bps,new_bps,before_value,after_value,reason,actor_id,request_id) VALUES((j->>'team_id')::uuid,(j->>'project_id')::uuid,(j->>'task_id')::uuid,(j->>'case_id')::uuid,(j->>'case_version_id')::uuid,(j->>'user_id')::uuid,(before_j->>'share_bps')::integer,(after_j->>'share_bps')::integer,before_j,after_j,j->>'change_reason',(j->>'changed_by')::uuid,j->>'request_id');
 PERFORM contribution_touch((j->>'case_id')::uuid,(j->>'changed_by')::uuid,j->>'change_reason',j->>'request_id');
 PERFORM contribution_emit((j->>'team_id')::uuid,(j->>'project_id')::uuid,(j->>'task_id')::uuid,(j->>'case_id')::uuid,(j->>'case_version_id')::uuid,'share_adjusted',(j->>'changed_by')::uuid,j->>'change_reason',j->>'request_id',jsonb_build_object('user_id',j->>'user_id','old_bps',before_j->'share_bps','new_bps',after_j->'share_bps')); RETURN NULL;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_shares_events AFTER INSERT OR UPDATE OR DELETE ON contribution_shares FOR EACH ROW EXECUTE FUNCTION contribution_share_event();
-- +goose StatementBegin
CREATE FUNCTION contribution_appraiser_guard() RETURNS trigger AS $$
DECLARE lead uuid; epoch integer; allowed boolean;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'authorization history cannot be deleted' USING ERRCODE='23514'; END IF;
 PERFORM contribution_lock(NEW.team_id,NEW.project_id,NULL);
 SELECT lead_id INTO lead FROM projects WHERE team_id=NEW.team_id AND id=NEW.project_id;
 IF NOT contribution_member(NEW.team_id,NEW.project_id,NEW.changed_by) THEN RAISE EXCEPTION 'authorization actor requires live project membership' USING ERRCODE='23514'; END IF;
 allowed:=NEW.changed_by=lead OR (NEW.user_id=lead AND (EXISTS(SELECT 1 FROM team_members WHERE team_id=NEW.team_id AND user_id=NEW.changed_by AND status='active' AND role IN ('owner','admin')) OR contribution_appraiser(NEW.team_id,NEW.project_id,NEW.changed_by))) OR (NEW.status='revoked' AND NEW.changed_by=NEW.user_id);
 IF NOT allowed THEN RAISE EXCEPTION 'no authority to grant or revoke appraisal permission' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' AND (NEW.team_id<>OLD.team_id OR NEW.project_id<>OLD.project_id OR NEW.user_id<>OLD.user_id OR NEW.version<>OLD.version+1) THEN RAISE EXCEPTION 'authorization scope and version are controlled' USING ERRCODE='23514'; END IF;
 IF NEW.status='active' THEN
  SELECT authorization_version INTO epoch FROM project_members WHERE team_id=NEW.team_id AND project_id=NEW.project_id AND user_id=NEW.user_id;
  IF NEW.user_id=NEW.changed_by OR NEW.granted_by<>NEW.changed_by OR NOT contribution_member(NEW.team_id,NEW.project_id,NEW.user_id) OR NEW.member_authorization_version IS DISTINCT FROM epoch THEN RAISE EXCEPTION 'self grant or stale qualification' USING ERRCODE='23514'; END IF;
  NEW.granted_at:=clock_timestamp();
 ELSIF TG_OP='INSERT' THEN RAISE EXCEPTION 'new authorization must be active' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_appraisers_guard BEFORE INSERT OR UPDATE OR DELETE ON contribution_appraisers FOR EACH ROW EXECUTE FUNCTION contribution_appraiser_guard();
-- +goose StatementBegin
CREATE FUNCTION contribution_appraiser_audit() RETURNS trigger AS $$
BEGIN
 INSERT INTO audit_events(actor_id,action,target_type,target_id,team_id,request_id,details) VALUES(NEW.changed_by,'contribution.appraiser_'||NEW.status,'project',NEW.project_id,NEW.team_id,NEW.request_id,jsonb_build_object('user_id',NEW.user_id,'epoch',NEW.member_authorization_version,'version',NEW.version,'reason',NEW.reason));RETURN NULL;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_appraisers_audit AFTER INSERT OR UPDATE ON contribution_appraisers FOR EACH ROW EXECUTE FUNCTION contribution_appraiser_audit();
-- +goose StatementBegin
CREATE FUNCTION contribution_confirmation_guard() RETURNS trigger AS $$
DECLARE c contribution_cases%ROWTYPE; v contribution_case_versions%ROWTYPE;
BEGIN
 PERFORM contribution_lock(NEW.team_id,NEW.project_id,NEW.case_id);
 SELECT * INTO c FROM contribution_cases WHERE id=NEW.case_id;
 SELECT * INTO v FROM contribution_case_versions WHERE id=NEW.case_version_id AND case_id=NEW.case_id AND team_id=NEW.team_id AND project_id=NEW.project_id AND task_id=NEW.task_id;
 IF v.status IS DISTINCT FROM 'pending_review' OR c.latest_version_id IS DISTINCT FROM v.id OR contribution_open_disputes(c.id) OR NOT COALESCE(contribution_independent(v.id,NEW.confirmed_by),false) OR NOT contribution_complete_shares(v.id) OR v.submitted_digest IS DISTINCT FROM contribution_digest(v.id) THEN RAISE EXCEPTION 'confirmation requires latest complete version and independent explicit appraiser' USING ERRCODE='23514'; END IF;
 IF NEW.version_digest<>'' AND NEW.version_digest IS DISTINCT FROM contribution_digest(v.id) THEN RAISE EXCEPTION 'incorrect confirmation digest' USING ERRCODE='23514'; END IF;
 NEW.version_digest:=contribution_digest(v.id);NEW.confirmed_at:=clock_timestamp();
 SELECT jsonb_object_agg(s.user_id::text,CASE WHEN contribution_member(s.team_id,s.project_id,s.user_id) THEN 'active' ELSE 'historical' END) INTO NEW.membership_snapshot FROM contribution_shares s WHERE s.case_version_id=v.id;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_confirmations_guard BEFORE INSERT ON contribution_confirmations FOR EACH ROW EXECUTE FUNCTION contribution_confirmation_guard();
-- +goose StatementBegin
CREATE FUNCTION contribution_confirmation_apply() RETURNS trigger AS $$
DECLARE previous uuid;
BEGIN
 SELECT effective_confirmed_version_id INTO previous FROM contribution_cases WHERE id=NEW.case_id;
 UPDATE contribution_case_versions SET status='confirmed',changed_by=NEW.confirmed_by,change_reason=NEW.rationale,request_id=NEW.request_id WHERE id=NEW.case_version_id;
 IF previous IS NOT NULL AND previous<>NEW.case_version_id THEN UPDATE contribution_case_versions SET status='superseded',changed_by=NEW.confirmed_by,change_reason=NEW.rationale,request_id=NEW.request_id WHERE id=previous; END IF;
 PERFORM contribution_emit(NEW.team_id,NEW.project_id,NEW.task_id,NEW.case_id,NEW.case_version_id,'confirmed',NEW.confirmed_by,NEW.rationale,NEW.request_id,jsonb_build_object('confirmation_id',NEW.id,'digest',NEW.version_digest)); RETURN NULL;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_confirmations_apply AFTER INSERT ON contribution_confirmations FOR EACH ROW EXECUTE FUNCTION contribution_confirmation_apply();
-- +goose StatementBegin
CREATE FUNCTION contribution_dispute_guard() RETURNS trigger AS $$
DECLARE c contribution_cases%ROWTYPE;
BEGIN
 PERFORM contribution_lock(NEW.team_id,NEW.project_id,NEW.case_id);
 SELECT * INTO c FROM contribution_cases WHERE id=NEW.case_id;
 IF NOT contribution_member(NEW.team_id,NEW.project_id,NEW.raised_by) OR c.effective_confirmed_version_id IS DISTINCT FROM NEW.case_version_id THEN RAISE EXCEPTION 'dispute requires active project actor and current effective confirmation' USING ERRCODE='23514'; END IF;
 IF NEW.on_behalf_of IS NOT NULL AND (NOT contribution_editor(NEW.team_id,NEW.project_id,NEW.raised_by) OR NOT EXISTS(SELECT 1 FROM contribution_attestations a WHERE a.id=NEW.identity_request_id AND a.team_id=NEW.team_id AND a.project_id=NEW.project_id AND a.task_id=NEW.task_id AND a.case_id=NEW.case_id AND a.case_version_id=NEW.case_version_id AND a.confirmation_id=NEW.confirmation_id AND a.subject_id=NEW.on_behalf_of AND a.purpose='dispute_request' AND a.content_digest=encode(sha256(convert_to(btrim(NEW.reason),'UTF8')),'hex'))) THEN RAISE EXCEPTION 'proxy dispute requires a verifiable authenticated request' USING ERRCODE='23514'; END IF;
 NEW.created_at:=clock_timestamp();RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_disputes_guard BEFORE INSERT ON contribution_disputes FOR EACH ROW EXECUTE FUNCTION contribution_dispute_guard();
-- +goose StatementBegin
CREATE FUNCTION contribution_dispute_event() RETURNS trigger AS $$
BEGIN
 PERFORM contribution_touch(NEW.case_id,NEW.raised_by,NEW.reason,NEW.request_id);
 PERFORM contribution_emit(NEW.team_id,NEW.project_id,NEW.task_id,NEW.case_id,NEW.case_version_id,'disputed',NEW.raised_by,NEW.reason,NEW.request_id,jsonb_build_object('dispute_id',NEW.id,'on_behalf_of',NEW.on_behalf_of));RETURN NULL;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_disputes_events AFTER INSERT ON contribution_disputes FOR EACH ROW EXECUTE FUNCTION contribution_dispute_event();
-- +goose StatementBegin
CREATE FUNCTION contribution_resolution_guard() RETURNS trigger AS $$
DECLARE d contribution_disputes%ROWTYPE; f contribution_confirmations%ROWTYPE; state text; effective uuid;
BEGIN
 PERFORM contribution_lock(NEW.team_id,NEW.project_id,NEW.case_id);
 SELECT * INTO d FROM contribution_disputes WHERE id=NEW.dispute_id AND team_id=NEW.team_id AND project_id=NEW.project_id AND task_id=NEW.task_id AND case_id=NEW.case_id;
 SELECT * INTO f FROM contribution_confirmations WHERE id=d.confirmation_id;
 SELECT status INTO state FROM contribution_case_versions WHERE id=d.case_version_id;
 SELECT effective_confirmed_version_id INTO effective FROM contribution_cases WHERE id=NEW.case_id;
 IF d.id IS NULL OR NOT COALESCE(contribution_independent(d.case_version_id,NEW.resolved_by),false) OR NEW.resolved_by=f.confirmed_by OR NEW.resolved_by=d.raised_by OR NEW.resolved_by=COALESCE(d.on_behalf_of,d.raised_by) OR NOT (effective=d.case_version_id AND state='confirmed' OR state='overturned' AND NEW.decision='overturn') THEN RAISE EXCEPTION 'resolution requires a different independent appraiser and consistent result' USING ERRCODE='23514'; END IF;
 NEW.resolved_at:=clock_timestamp();RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_resolutions_guard BEFORE INSERT ON contribution_dispute_resolutions FOR EACH ROW EXECUTE FUNCTION contribution_resolution_guard();
-- +goose StatementBegin
CREATE FUNCTION contribution_resolution_apply() RETURNS trigger AS $$
DECLARE d contribution_disputes%ROWTYPE; state text; v contribution_case_versions%ROWTYPE; n integer;
BEGIN
 SELECT * INTO d FROM contribution_disputes WHERE id=NEW.dispute_id;
 SELECT status INTO state FROM contribution_case_versions WHERE id=d.case_version_id;
 IF NEW.decision='overturn' AND state='confirmed' THEN
  FOR v IN SELECT * FROM contribution_case_versions WHERE case_id=NEW.case_id AND status IN ('draft','pending_review') LOOP
   UPDATE contribution_case_versions SET status='abandoned',abandonment_resolution_id=NEW.id,changed_by=NEW.resolved_by,change_reason=NEW.reason,request_id=NEW.request_id WHERE id=v.id;
  END LOOP;
  UPDATE contribution_case_versions SET status='overturned',changed_by=NEW.resolved_by,change_reason=NEW.reason,request_id=NEW.request_id WHERE id=d.case_version_id;
  SELECT COALESCE(max(version),0)+1 INTO n FROM contribution_case_versions WHERE case_id=NEW.case_id;
  INSERT INTO contribution_case_versions(team_id,project_id,task_id,case_id,version,supersedes_version_id,proposed_by,changed_by,change_reason,request_id) VALUES(NEW.team_id,NEW.project_id,NEW.task_id,NEW.case_id,n,d.case_version_id,NEW.resolved_by,NEW.resolved_by,NEW.reason,NEW.request_id);
 END IF;
 PERFORM contribution_touch(NEW.case_id,NEW.resolved_by,NEW.reason,NEW.request_id);
 PERFORM contribution_emit(NEW.team_id,NEW.project_id,NEW.task_id,NEW.case_id,d.case_version_id,'resolution_'||NEW.decision,NEW.resolved_by,NEW.reason,NEW.request_id,jsonb_build_object('resolution_id',NEW.id,'dispute_id',d.id)); RETURN NULL;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_resolutions_apply AFTER INSERT ON contribution_dispute_resolutions FOR EACH ROW EXECUTE FUNCTION contribution_resolution_apply();
-- +goose StatementBegin
CREATE FUNCTION contribution_notice_guard() RETURNS trigger AS $$
BEGIN
 PERFORM contribution_lock(NEW.team_id,NEW.project_id,NEW.case_id);
 IF NOT contribution_editor(NEW.team_id,NEW.project_id,NEW.created_by) OR NOT EXISTS(SELECT 1 FROM contribution_shares WHERE case_version_id=NEW.case_version_id AND user_id=NEW.recipient_id) THEN RAISE EXCEPTION 'notice requires an editor and an actual share recipient' USING ERRCODE='23514'; END IF;
 NEW.prepared_at:=clock_timestamp();RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_notices_guard BEFORE INSERT ON contribution_notifications FOR EACH ROW EXECUTE FUNCTION contribution_notice_guard();
-- +goose StatementBegin
CREATE FUNCTION contribution_attestation_guard() RETURNS trigger AS $$
DECLARE s sessions%ROWTYPE; n contribution_notifications%ROWTYPE; digest text;
BEGIN
 PERFORM contribution_lock(NEW.team_id,NEW.project_id,NEW.case_id);
 IF length(COALESCE(NEW.proof_token,'')) NOT BETWEEN 1 AND 512 THEN RAISE EXCEPTION 'bounded authentication proof required' USING ERRCODE='23514'; END IF;
 SELECT * INTO s FROM sessions WHERE user_id=NEW.subject_id AND token_hash=encode(sha256(convert_to(COALESCE(NEW.proof_token,''),'UTF8')),'hex') AND revoked_at IS NULL AND expires_at>clock_timestamp() FOR SHARE;
 IF NOT FOUND THEN RAISE EXCEPTION 'verifiable subject authentication required' USING ERRCODE='23514'; END IF;
 NEW.session_id:=s.id;NEW.proof_token:=NULL;NEW.verified_at:=clock_timestamp();
 IF NEW.purpose='delivery' THEN
  SELECT * INTO n FROM contribution_notifications WHERE id=NEW.notice_id AND team_id=NEW.team_id AND project_id=NEW.project_id AND task_id=NEW.task_id AND case_id=NEW.case_id AND case_version_id=NEW.case_version_id AND confirmation_id=NEW.confirmation_id;
  SELECT version_digest INTO digest FROM contribution_confirmations WHERE id=NEW.confirmation_id;
  IF n.id IS NULL OR n.channel<>'authenticated_ack' OR n.recipient_id<>NEW.subject_id OR NEW.content_digest IS DISTINCT FROM digest THEN RAISE EXCEPTION 'receipt must acknowledge the actual recipient and confirmed result digest' USING ERRCODE='23514'; END IF;
  NEW.sent_at:=NEW.verified_at;NEW.acknowledged_at:=NEW.verified_at;
 ELSE
  IF NEW.notice_id IS NOT NULL OR length(btrim(NEW.statement))=0 OR NEW.content_digest<>encode(sha256(convert_to(btrim(NEW.statement),'UTF8')),'hex') THEN RAISE EXCEPTION 'identity request must bind its exact statement' USING ERRCODE='23514'; END IF;
  NEW.sent_at:=NULL;NEW.acknowledged_at:=NULL;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_attestations_guard BEFORE INSERT ON contribution_attestations FOR EACH ROW EXECUTE FUNCTION contribution_attestation_guard();
-- +goose StatementBegin
CREATE FUNCTION contribution_fact_guard() RETURNS trigger AS $$
DECLARE j jsonb;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'contribution facts are append only' USING ERRCODE='23514'; END IF;
 j:=to_jsonb(NEW);PERFORM contribution_lock((j->>'team_id')::uuid,(j->>'project_id')::uuid,(j->>'case_id')::uuid);
 IF TG_TABLE_NAME IN ('contribution_case_events','contribution_share_adjustments') AND pg_trigger_depth()<2 THEN RAISE EXCEPTION 'facts are generated by the owning database mutation' USING ERRCODE='23514'; END IF;
 IF TG_TABLE_NAME='contribution_conflicts' AND NOT (contribution_member(NEW.team_id,NEW.project_id,(j->>'recorded_by')::uuid) AND ((j->>'recorded_by')::uuid=(j->>'user_id')::uuid OR contribution_editor(NEW.team_id,NEW.project_id,(j->>'recorded_by')::uuid))) THEN RAISE EXCEPTION 'conflict declaration actor not authorized' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
-- +goose StatementBegin
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['contribution_share_adjustments','contribution_confirmations','contribution_conflicts','contribution_disputes','contribution_dispute_resolutions','contribution_case_events','contribution_notifications','contribution_attestations'] LOOP
  EXECUTE format('CREATE TRIGGER contribution_facts_guard BEFORE INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION contribution_fact_guard()',t);
 END LOOP;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_misc_event() RETURNS trigger AS $$
DECLARE j jsonb; actor uuid;
BEGIN
 j:=to_jsonb(NEW);actor:=COALESCE((j->>'created_by')::uuid,(j->>'recorded_by')::uuid,(j->>'subject_id')::uuid);
 PERFORM contribution_touch(NEW.case_id,actor,'contribution evidence recorded',NEW.request_id);
 PERFORM contribution_emit(NEW.team_id,NEW.project_id,NEW.task_id,NEW.case_id,(j->>'case_version_id')::uuid,TG_TABLE_NAME,actor,'contribution evidence recorded',NEW.request_id,jsonb_build_object('id',NEW.id,'purpose',j->>'purpose'));RETURN NULL;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_notices_event AFTER INSERT ON contribution_notifications FOR EACH ROW EXECUTE FUNCTION contribution_misc_event();
CREATE TRIGGER contribution_attestations_event AFTER INSERT ON contribution_attestations FOR EACH ROW EXECUTE FUNCTION contribution_misc_event();
CREATE TRIGGER contribution_conflicts_event AFTER INSERT ON contribution_conflicts FOR EACH ROW EXECUTE FUNCTION contribution_misc_event();
-- +goose StatementBegin
CREATE FUNCTION contribution_validate_case(p_case uuid) RETURNS void AS $$
DECLARE c contribution_cases%ROWTYPE; v contribution_case_versions%ROWTYPE; latest uuid; effective uuid; n integer; state text;
BEGIN
 SELECT * INTO c FROM contribution_cases WHERE id=p_case;
 IF NOT FOUND THEN RETURN; END IF;
 SELECT * INTO v FROM contribution_case_versions WHERE case_id=p_case ORDER BY version DESC LIMIT 1;
 latest:=v.id;
 SELECT count(*),min(id::text)::uuid INTO n,effective FROM contribution_case_versions WHERE case_id=p_case AND status='confirmed';
 IF latest IS NULL OR c.latest_version_id IS DISTINCT FROM latest OR n>1 OR c.effective_confirmed_version_id IS DISTINCT FROM effective THEN RAISE EXCEPTION 'contribution version pointers disagree with authoritative facts' USING ERRCODE='23514'; END IF;
 state:=CASE WHEN contribution_open_disputes(p_case) THEN 'disputed' WHEN effective IS NOT NULL AND latest<>effective AND v.status IN ('draft','pending_review') THEN 'revising' WHEN effective IS NOT NULL THEN 'confirmed' WHEN v.status='pending_review' THEN 'pending_review' WHEN v.status='abandoned' THEN 'abandoned' ELSE 'draft' END;
 IF c.case_workflow_status<>state THEN RAISE EXCEPTION 'incorrect case workflow' USING ERRCODE='23514'; END IF;
 IF NOT EXISTS(SELECT 1 FROM tasks t JOIN task_reviews r ON r.id=c.review_id AND r.team_id=c.team_id AND r.project_id=c.project_id AND r.task_id=c.task_id AND r.submission_version_id=c.submission_version_id WHERE t.id=c.task_id AND t.team_id=c.team_id AND t.project_id=c.project_id AND t.status='completed' AND t.current_submission_version_id=c.submission_version_id AND t.accepted_review_id=c.review_id AND r.decision='approve') THEN RAISE EXCEPTION 'current accepted source no longer matches' USING ERRCODE='23514'; END IF;
 FOR v IN SELECT * FROM contribution_case_versions WHERE case_id=p_case LOOP
  IF v.status IN ('pending_review','confirmed','superseded','overturned') AND (NOT contribution_complete_shares(v.id) OR v.submitted_digest IS DISTINCT FROM contribution_digest(v.id)) THEN RAISE EXCEPTION 'frozen contribution content or 10000 bps is invalid' USING ERRCODE='23514'; END IF;
  IF v.status IN ('confirmed','superseded','overturned') AND NOT EXISTS(SELECT 1 FROM contribution_confirmations f WHERE f.case_version_id=v.id AND f.version_digest=contribution_digest(v.id)) THEN RAISE EXCEPTION 'immutable formal confirmation required' USING ERRCODE='23514'; END IF;
  IF v.status NOT IN ('confirmed','superseded','overturned') AND EXISTS(SELECT 1 FROM contribution_confirmations WHERE case_version_id=v.id) THEN RAISE EXCEPTION 'confirmation cannot return to draft' USING ERRCODE='23514'; END IF;
  IF v.status='superseded' AND NOT EXISTS(SELECT 1 FROM contribution_confirmations f JOIN contribution_case_versions newer ON newer.id=f.case_version_id WHERE newer.case_id=p_case AND newer.version>v.version) THEN RAISE EXCEPTION 'superseded requires a later confirmation fact' USING ERRCODE='23514'; END IF;
  IF v.status='overturned' AND (c.effective_confirmed_version_id=v.id OR NOT EXISTS(SELECT 1 FROM contribution_disputes d JOIN contribution_dispute_resolutions r ON r.dispute_id=d.id WHERE d.case_version_id=v.id AND r.decision='overturn') OR NOT EXISTS(SELECT 1 FROM contribution_case_versions child WHERE child.case_id=p_case AND child.supersedes_version_id=v.id AND child.version>v.version)) THEN RAISE EXCEPTION 'overturn must retain ruling and successor and clear original effectiveness' USING ERRCODE='23514'; END IF;
 END LOOP;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_constraint_check() RETURNS trigger AS $$
DECLARE j jsonb; id uuid;
BEGIN
 j:=CASE WHEN TG_OP='DELETE' THEN to_jsonb(OLD) ELSE to_jsonb(NEW) END;
 id:=CASE WHEN TG_TABLE_NAME='contribution_cases' THEN (j->>'id')::uuid ELSE (j->>'case_id')::uuid END;
 PERFORM contribution_validate_case(id);RETURN NULL;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
-- +goose StatementBegin
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['contribution_cases','contribution_case_versions','contribution_shares','contribution_confirmations','contribution_disputes','contribution_dispute_resolutions'] LOOP
  EXECUTE format('CREATE CONSTRAINT TRIGGER contribution_final_integrity AFTER INSERT OR UPDATE OR DELETE ON %I DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION contribution_constraint_check()',t);
 END LOOP;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_membership_lock() RETURNS trigger AS $$
DECLARE j jsonb;
BEGIN
 j:=CASE WHEN TG_OP='DELETE' THEN to_jsonb(OLD) ELSE to_jsonb(NEW) END;
 PERFORM 1 FROM teams WHERE id=(j->>'team_id')::uuid FOR UPDATE;
 IF TG_OP='DELETE' THEN
  -- Preserve the established Phase02 last-owner failure before rejecting
  -- other physical deletions of membership history.
  IF TG_TABLE_NAME='team_members' AND j->>'role'='owner' AND j->>'status'='active' AND NOT EXISTS(SELECT 1 FROM team_members WHERE team_id=(j->>'team_id')::uuid AND user_id<>(j->>'user_id')::uuid AND status='active' AND role='owner') THEN
   RAISE EXCEPTION 'team must retain at least one active owner';
  END IF;
  RAISE EXCEPTION 'membership history uses soft removal' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_qualification_lock BEFORE UPDATE OR DELETE ON project_members FOR EACH ROW EXECUTE FUNCTION contribution_membership_lock();
CREATE TRIGGER contribution_qualification_lock BEFORE UPDATE OR DELETE ON team_members FOR EACH ROW EXECUTE FUNCTION contribution_membership_lock();
-- +goose StatementBegin
CREATE FUNCTION contribution_archive_guard() RETURNS trigger AS $$
BEGIN
 IF OLD.status='active' AND NEW.status='archived' THEN
  PERFORM contribution_lock(NEW.team_id,NEW.id,NULL);
  PERFORM 1 FROM contribution_cases WHERE team_id=NEW.team_id AND project_id=NEW.id ORDER BY id FOR UPDATE;
  IF EXISTS(
   SELECT 1 FROM tasks t JOIN task_reviews r ON r.team_id=t.team_id AND r.project_id=t.project_id AND r.task_id=t.id AND r.submission_version_id=t.current_submission_version_id AND r.id=t.accepted_review_id AND r.decision='approve'
   LEFT JOIN contribution_cases c ON c.team_id=t.team_id AND c.project_id=t.project_id AND c.task_id=t.id AND c.review_id=r.id AND c.submission_version_id=t.current_submission_version_id
   LEFT JOIN contribution_case_versions v ON v.id=c.effective_confirmed_version_id AND v.case_id=c.id AND v.status='confirmed'
   LEFT JOIN contribution_confirmations f ON f.case_version_id=v.id
   WHERE t.team_id=NEW.team_id AND t.project_id=NEW.id AND t.status='completed' AND
   (c.id IS NULL OR v.id IS NULL OR f.id IS NULL OR c.case_workflow_status<>'confirmed' OR contribution_open_disputes(c.id) OR EXISTS(SELECT 1 FROM contribution_case_versions working WHERE working.case_id=c.id AND working.status IN ('draft','pending_review')) OR EXISTS(
    SELECT 1 FROM contribution_shares s WHERE s.case_version_id=v.id AND NOT EXISTS(
     SELECT 1 FROM contribution_notifications notice JOIN contribution_attestations proof ON proof.notice_id=notice.id AND proof.purpose='delivery' AND proof.subject_id=s.user_id AND proof.confirmation_id=f.id AND proof.case_version_id=v.id AND proof.content_digest=f.version_digest AND proof.sent_at IS NOT NULL AND proof.acknowledged_at IS NOT NULL
     WHERE notice.team_id=c.team_id AND notice.project_id=c.project_id AND notice.task_id=c.task_id AND notice.case_id=c.id AND notice.case_version_id=v.id AND notice.confirmation_id=f.id AND notice.recipient_id=s.user_id AND notice.channel='authenticated_ack'
    )
   ))
  ) THEN RAISE EXCEPTION 'all accepted tasks require settled contributions and verifiable result delivery' USING ERRCODE='23514',CONSTRAINT='contribution_settlement_incomplete'; END IF;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER contribution_archive_integrity BEFORE UPDATE OF status ON projects FOR EACH ROW EXECUTE FUNCTION contribution_archive_guard();
-- SQL commands are transaction helpers, not HTTP APIs. M3 must authenticate
-- actor identities and scope, then call these under its normal service policy.
-- +goose StatementBegin
CREATE FUNCTION contribution_set_appraiser(p_team uuid,p_project uuid,p_user uuid,p_actor uuid,p_reason text,p_request text,p_expected integer,p_revoke boolean) RETURNS void AS $$
DECLARE epoch integer; old_version integer;
BEGIN
 PERFORM contribution_lock(p_team,p_project,NULL);
 SELECT version INTO old_version FROM contribution_appraisers WHERE team_id=p_team AND project_id=p_project AND user_id=p_user;
 IF COALESCE(old_version,0) IS DISTINCT FROM p_expected THEN RAISE EXCEPTION 'authorization expected_version conflict' USING ERRCODE='40001'; END IF;
 IF p_revoke THEN
  UPDATE contribution_appraisers SET status='revoked',changed_by=p_actor,reason=p_reason,request_id=p_request,version=version+1 WHERE team_id=p_team AND project_id=p_project AND user_id=p_user AND status='active';
  IF NOT FOUND THEN RAISE EXCEPTION 'active appraisal authorization not found' USING ERRCODE='23514'; END IF;
 ELSE
  SELECT authorization_version INTO epoch FROM project_members WHERE team_id=p_team AND project_id=p_project AND user_id=p_user;
  INSERT INTO contribution_appraisers(team_id,project_id,user_id,member_authorization_version,status,granted_by,changed_by,reason,request_id) VALUES(p_team,p_project,p_user,epoch,'active',p_actor,p_actor,p_reason,p_request)
  ON CONFLICT(team_id,project_id,user_id) DO UPDATE SET member_authorization_version=EXCLUDED.member_authorization_version,status='active',granted_by=EXCLUDED.granted_by,changed_by=EXCLUDED.changed_by,reason=EXCLUDED.reason,request_id=EXCLUDED.request_id,version=contribution_appraisers.version+1;
 END IF;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_create_case(p_team uuid,p_project uuid,p_task uuid,p_review uuid,p_actor uuid,p_request text) RETURNS uuid AS $$
DECLARE result uuid; source uuid;
BEGIN
 PERFORM contribution_lock(p_team,p_project,NULL);
 IF NOT contribution_editor(p_team,p_project,p_actor) THEN RAISE EXCEPTION 'case creation needs a project editor' USING ERRCODE='23514'; END IF;
 SELECT r.submission_version_id INTO source FROM task_reviews r JOIN tasks t ON t.id=r.task_id AND t.team_id=r.team_id AND t.project_id=r.project_id WHERE r.team_id=p_team AND r.project_id=p_project AND r.task_id=p_task AND r.id=p_review AND r.decision='approve' AND t.status='completed' AND t.accepted_review_id=r.id AND t.current_submission_version_id=r.submission_version_id;
 IF source IS NULL THEN RAISE EXCEPTION 'current formal approval not found' USING ERRCODE='23514'; END IF;
 INSERT INTO contribution_cases(team_id,project_id,task_id,submission_version_id,review_id,created_by,last_actor_id,last_reason,request_id) VALUES(p_team,p_project,p_task,source,p_review,p_actor,p_actor,'create from formal acceptance',p_request) ON CONFLICT(review_id) DO NOTHING RETURNING id INTO result;
 IF result IS NULL THEN SELECT id INTO result FROM contribution_cases WHERE team_id=p_team AND project_id=p_project AND task_id=p_task AND review_id=p_review; END IF;
 RETURN result;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_set_share(p_team uuid,p_project uuid,p_case uuid,p_version uuid,p_user uuid,p_bps integer,p_basis text,p_statement text,p_evidence jsonb,p_actor uuid,p_expected integer,p_reason text,p_request text) RETURNS void AS $$
DECLARE c contribution_cases%ROWTYPE;
BEGIN
 PERFORM contribution_expected(p_team,p_project,p_case,p_expected);
 SELECT * INTO c FROM contribution_cases WHERE id=p_case;
 IF c.latest_version_id<>p_version OR NOT contribution_editor(p_team,p_project,p_actor) THEN RAISE EXCEPTION 'only latest draft can be edited' USING ERRCODE='23514'; END IF;
 IF p_bps=0 THEN
  UPDATE contribution_shares SET changed_by=p_actor,change_reason=p_reason,request_id=p_request WHERE team_id=p_team AND project_id=p_project AND case_id=p_case AND case_version_id=p_version AND user_id=p_user;
  DELETE FROM contribution_shares WHERE team_id=p_team AND project_id=p_project AND case_id=p_case AND case_version_id=p_version AND user_id=p_user;
 ELSE
  INSERT INTO contribution_shares(team_id,project_id,task_id,case_id,case_version_id,user_id,share_bps,basis,statement,evidence_refs,changed_by,change_reason,request_id) VALUES(p_team,p_project,c.task_id,p_case,p_version,p_user,p_bps,p_basis,p_statement,p_evidence,p_actor,p_reason,p_request)
  ON CONFLICT(case_version_id,user_id) DO UPDATE SET share_bps=EXCLUDED.share_bps,basis=EXCLUDED.basis,statement=EXCLUDED.statement,evidence_refs=EXCLUDED.evidence_refs,changed_by=EXCLUDED.changed_by,change_reason=EXCLUDED.change_reason,request_id=EXCLUDED.request_id;
 END IF;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_transition(p_team uuid,p_project uuid,p_case uuid,p_actor uuid,p_expected integer,p_to text,p_reason text,p_request text) RETURNS void AS $$
DECLARE target_version uuid;
BEGIN
 PERFORM contribution_expected(p_team,p_project,p_case,p_expected);
 SELECT latest_version_id INTO target_version FROM contribution_cases WHERE team_id=p_team AND project_id=p_project AND id=p_case;
 IF p_to NOT IN ('draft','pending_review','abandoned') THEN RAISE EXCEPTION 'formal states require their own evidence' USING ERRCODE='23514'; END IF;
 UPDATE contribution_case_versions SET status=p_to,changed_by=p_actor,change_reason=p_reason,request_id=p_request WHERE id=target_version;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_confirm(p_team uuid,p_project uuid,p_case uuid,p_actor uuid,p_expected integer,p_reason text,p_request text) RETURNS uuid AS $$
DECLARE c contribution_cases%ROWTYPE; result uuid;
BEGIN
 PERFORM contribution_expected(p_team,p_project,p_case,p_expected);SELECT * INTO c FROM contribution_cases WHERE id=p_case;
 INSERT INTO contribution_confirmations(team_id,project_id,task_id,case_id,case_version_id,confirmed_by,rationale,request_id) VALUES(p_team,p_project,c.task_id,p_case,c.latest_version_id,p_actor,p_reason,p_request) RETURNING id INTO result;RETURN result;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_revise(p_team uuid,p_project uuid,p_case uuid,p_actor uuid,p_expected integer,p_reason text,p_request text) RETURNS uuid AS $$
DECLARE c contribution_cases%ROWTYPE; n integer; result uuid;
BEGIN
 PERFORM contribution_expected(p_team,p_project,p_case,p_expected);SELECT * INTO c FROM contribution_cases WHERE id=p_case;
 SELECT max(version)+1 INTO n FROM contribution_case_versions WHERE case_id=p_case;
 INSERT INTO contribution_case_versions(team_id,project_id,task_id,case_id,version,supersedes_version_id,proposed_by,changed_by,change_reason,request_id) VALUES(p_team,p_project,c.task_id,p_case,n,c.latest_version_id,p_actor,p_actor,p_reason,p_request) RETURNING id INTO result;RETURN result;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_raise_dispute(p_team uuid,p_project uuid,p_case uuid,p_confirmation uuid,p_actor uuid,p_expected integer,p_reason text,p_request text,p_on_behalf uuid,p_identity_request uuid) RETURNS uuid AS $$
DECLARE c contribution_cases%ROWTYPE; v uuid; result uuid;
BEGIN
 PERFORM contribution_expected(p_team,p_project,p_case,p_expected);SELECT * INTO c FROM contribution_cases WHERE id=p_case;
 SELECT case_version_id INTO v FROM contribution_confirmations WHERE id=p_confirmation AND case_id=p_case;
 INSERT INTO contribution_disputes(team_id,project_id,task_id,case_id,case_version_id,confirmation_id,raised_by,on_behalf_of,identity_request_id,verification_method,verification_evidence_ref,reason,request_id) VALUES(p_team,p_project,c.task_id,p_case,v,p_confirmation,p_actor,p_on_behalf,p_identity_request,CASE WHEN p_on_behalf IS NULL THEN '' ELSE 'authenticated_request' END,COALESCE(p_identity_request::text,''),p_reason,p_request) RETURNING id INTO result;RETURN result;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION contribution_resolve(p_team uuid,p_project uuid,p_case uuid,p_dispute uuid,p_actor uuid,p_expected integer,p_decision text,p_reason text,p_request text) RETURNS uuid AS $$
DECLARE c contribution_cases%ROWTYPE; result uuid;
BEGIN
 PERFORM contribution_expected(p_team,p_project,p_case,p_expected);SELECT * INTO c FROM contribution_cases WHERE id=p_case;
 INSERT INTO contribution_dispute_resolutions(team_id,project_id,task_id,case_id,dispute_id,decision,resolved_by,reason,request_id) VALUES(p_team,p_project,c.task_id,p_case,p_dispute,p_decision,p_actor,p_reason,p_request) RETURNING id INTO result;RETURN result;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE VIEW contribution_effective_shares AS
 SELECT c.team_id,c.project_id,c.task_id,c.id AS case_id,v.id AS case_version_id,f.id AS confirmation_id,c.case_workflow_status,s.user_id,s.share_bps,f.membership_snapshot->>s.user_id::text AS membership_at_confirmation
 FROM contribution_cases c JOIN contribution_case_versions v ON v.id=c.effective_confirmed_version_id AND v.case_id=c.id AND v.status='confirmed' JOIN contribution_confirmations f ON f.case_version_id=v.id JOIN contribution_shares s ON s.case_version_id=v.id
 WHERE NOT contribution_open_disputes(c.id);
CREATE VIEW contribution_version_history AS
 SELECT v.*,f.id AS confirmation_id,f.confirmed_by,f.confirmed_at,c.latest_version_id=v.id AS is_latest,c.effective_confirmed_version_id=v.id AS is_effective
 FROM contribution_case_versions v JOIN contribution_cases c ON c.id=v.case_id LEFT JOIN contribution_confirmations f ON f.case_version_id=v.id;
CREATE VIEW contribution_member_coverage AS
 WITH historical AS (
  SELECT team_id,project_id,user_id FROM project_members
  UNION SELECT v.team_id,v.project_id,v.author_id FROM task_submission_versions v JOIN task_acceptance_evidence e ON e.team_id=v.team_id AND e.project_id=v.project_id AND e.task_id=v.task_id AND e.current_version_id=v.id
  UNION SELECT a.team_id,a.project_id,a.uploader_id FROM submission_attachments a JOIN task_acceptance_evidence e ON e.team_id=a.team_id AND e.project_id=a.project_id AND e.task_id=a.task_id AND e.current_version_id=a.submission_version_id WHERE a.status='bound'
 ) SELECT h.*,contribution_member(h.team_id,h.project_id,h.user_id) AS currently_active,EXISTS(SELECT 1 FROM contribution_effective_shares s WHERE s.team_id=h.team_id AND s.project_id=h.project_id AND s.user_id=h.user_id) AS has_effective_share FROM historical h;
CREATE VIEW contribution_project_counts AS
 SELECT p.team_id,p.id AS project_id,
 (SELECT count(*) FROM contribution_cases c WHERE c.team_id=p.team_id AND c.project_id=p.id) AS case_count,
 (SELECT count(DISTINCT s.case_id) FROM contribution_effective_shares s WHERE s.team_id=p.team_id AND s.project_id=p.id) AS confirmed_case_count,
 (SELECT count(*) FROM contribution_case_versions v WHERE v.team_id=p.team_id AND v.project_id=p.id AND v.status='pending_review') AS pending_count,
 (SELECT count(*) FROM contribution_cases c WHERE c.team_id=p.team_id AND c.project_id=p.id AND c.case_workflow_status='revising') AS revising_count,
 (SELECT count(*) FROM contribution_disputes d WHERE d.team_id=p.team_id AND d.project_id=p.id AND NOT EXISTS(SELECT 1 FROM contribution_dispute_resolutions r WHERE r.dispute_id=d.id)) AS unresolved_dispute_count,
 (SELECT count(*) FROM contribution_case_versions v WHERE v.team_id=p.team_id AND v.project_id=p.id AND v.status='overturned') AS overturned_version_count,
 (SELECT count(*) FROM contribution_case_versions v WHERE v.team_id=p.team_id AND v.project_id=p.id AND v.status='superseded') AS superseded_version_count
 FROM projects p;


-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM contribution_cases) OR EXISTS(SELECT 1 FROM contribution_appraisers) THEN RAISE EXCEPTION 'cannot roll back contribution business or governance evidence'; END IF;
END $$;
-- +goose StatementEnd
DROP VIEW contribution_project_counts,contribution_member_coverage,contribution_version_history,contribution_effective_shares;
-- +goose StatementBegin
DO $$ DECLARE r record; BEGIN
 FOR r IN SELECT t.tgname,t.tgrelid::regclass AS rel FROM pg_trigger t JOIN pg_proc p ON p.oid=t.tgfoid WHERE p.pronamespace=current_schema()::regnamespace AND p.proname LIKE 'contribution_%' AND NOT t.tgisinternal LOOP
  EXECUTE format('DROP TRIGGER %I ON %s',r.tgname,r.rel);
 END LOOP;
END $$;
-- +goose StatementEnd
DROP FUNCTION contribution_resolve(uuid,uuid,uuid,uuid,uuid,integer,text,text,text);
DROP FUNCTION contribution_raise_dispute(uuid,uuid,uuid,uuid,uuid,integer,text,text,uuid,uuid);
DROP FUNCTION contribution_revise(uuid,uuid,uuid,uuid,integer,text,text);
DROP FUNCTION contribution_confirm(uuid,uuid,uuid,uuid,integer,text,text);
DROP FUNCTION contribution_transition(uuid,uuid,uuid,uuid,integer,text,text,text);
DROP FUNCTION contribution_set_share(uuid,uuid,uuid,uuid,uuid,integer,text,text,jsonb,uuid,integer,text,text);
DROP FUNCTION contribution_create_case(uuid,uuid,uuid,uuid,uuid,text);
DROP FUNCTION contribution_set_appraiser(uuid,uuid,uuid,uuid,text,text,integer,boolean);
DROP FUNCTION contribution_archive_guard();
DROP FUNCTION contribution_membership_lock();
DROP FUNCTION contribution_constraint_check();
DROP FUNCTION contribution_validate_case(uuid);
DROP FUNCTION contribution_misc_event();
DROP FUNCTION contribution_fact_guard();
DROP FUNCTION contribution_attestation_guard();
DROP FUNCTION contribution_notice_guard();
DROP FUNCTION contribution_resolution_apply();
DROP FUNCTION contribution_resolution_guard();
DROP FUNCTION contribution_dispute_event();
DROP FUNCTION contribution_dispute_guard();
DROP FUNCTION contribution_confirmation_apply();
DROP FUNCTION contribution_confirmation_guard();
DROP FUNCTION contribution_appraiser_audit();
DROP FUNCTION contribution_appraiser_guard();
DROP FUNCTION contribution_share_event();
DROP FUNCTION contribution_share_guard();
DROP FUNCTION contribution_version_event();
DROP FUNCTION contribution_version_guard();
DROP FUNCTION contribution_case_created();
DROP FUNCTION contribution_case_guard();
DROP FUNCTION contribution_independent(uuid,uuid);
DROP FUNCTION contribution_complete_shares(uuid);
DROP FUNCTION contribution_digest(uuid);
DROP FUNCTION contribution_open_disputes(uuid);
DROP FUNCTION contribution_touch(uuid,uuid,text,text);
DROP FUNCTION contribution_event_audit();
DROP FUNCTION contribution_emit(uuid,uuid,uuid,uuid,uuid,text,uuid,text,text,jsonb);
DROP FUNCTION contribution_expected(uuid,uuid,uuid,integer);
DROP FUNCTION contribution_lock(uuid,uuid,uuid);
DROP FUNCTION contribution_editor(uuid,uuid,uuid);
DROP FUNCTION contribution_appraiser(uuid,uuid,uuid);
DROP FUNCTION contribution_member(uuid,uuid,uuid);
ALTER TABLE contribution_disputes DROP CONSTRAINT contribution_identity_request_fk;
ALTER TABLE contribution_case_versions DROP CONSTRAINT contribution_retirement_ruling_fk;
ALTER TABLE contribution_cases DROP CONSTRAINT contribution_latest_fk;
ALTER TABLE contribution_cases DROP CONSTRAINT contribution_effective_fk;
DROP TABLE contribution_attestations,contribution_notifications,contribution_case_events,contribution_dispute_resolutions,contribution_disputes,contribution_conflicts,contribution_confirmations,contribution_share_adjustments,contribution_shares,contribution_case_versions,contribution_cases,contribution_appraisers;
