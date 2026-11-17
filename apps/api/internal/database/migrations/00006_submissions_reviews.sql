-- +goose Up
ALTER TABLE project_members ADD COLUMN authorization_version integer NOT NULL DEFAULT 1 CHECK(authorization_version>0);
-- +goose StatementBegin
CREATE FUNCTION project_members_bump_authorization() RETURNS trigger AS $$
BEGIN
 IF NEW.status IS DISTINCT FROM OLD.status THEN NEW.authorization_version=OLD.authorization_version+1; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER project_members_authorization_guard BEFORE UPDATE OF status ON project_members FOR EACH ROW EXECUTE FUNCTION project_members_bump_authorization();

CREATE TABLE project_reviewers (
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 user_id uuid NOT NULL,
 member_authorization_version integer NOT NULL CHECK(member_authorization_version>0),
 status text NOT NULL CHECK(status IN ('active','revoked')),
 granted_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 granted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 1000),
 version integer NOT NULL DEFAULT 1 CHECK(version>0),
 PRIMARY KEY(team_id,project_id,user_id),
 FOREIGN KEY(team_id,project_id,user_id) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT
);
CREATE INDEX project_reviewers_user_idx ON project_reviewers(team_id,user_id,status);
-- +goose StatementBegin
CREATE FUNCTION phase04_can_review(tid uuid,pid uuid,uid uuid) RETURNS boolean AS $$
 SELECT EXISTS(
  SELECT 1 FROM projects p JOIN project_members pm ON pm.team_id=p.team_id AND pm.project_id=p.id AND pm.user_id=uid AND pm.status='active'
  JOIN team_members tm ON tm.team_id=pm.team_id AND tm.user_id=pm.user_id AND tm.status='active'
  LEFT JOIN project_reviewers g ON g.team_id=pm.team_id AND g.project_id=pm.project_id AND g.user_id=pm.user_id
  WHERE p.team_id=tid AND p.id=pid AND (p.lead_id=uid OR (g.status='active' AND g.member_authorization_version=pm.authorization_version))
 );
$$ LANGUAGE sql STABLE;
-- +goose StatementEnd

CREATE TABLE task_submissions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 task_id uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(team_id,project_id,task_id),
 UNIQUE(team_id,project_id,task_id,id),
 FOREIGN KEY(team_id,project_id,task_id) REFERENCES tasks(team_id,project_id,id) ON DELETE RESTRICT
);
CREATE TABLE task_submission_versions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 task_id uuid NOT NULL,
 submission_id uuid NOT NULL,
 version integer NOT NULL CHECK(version>0),
 author_id uuid NOT NULL,
 summary text NOT NULL DEFAULT '' CHECK(length(summary)<=8000),
 evidence_references jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(evidence_references)='array' AND jsonb_array_length(evidence_references)<=20),
 client_request_id uuid NOT NULL,
 content_hash text NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
 based_on_task_version integer NOT NULL CHECK(based_on_task_version>0),
 creation_xid xid8 NOT NULL DEFAULT pg_current_xact_id(),
 submitted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 request_id text NOT NULL DEFAULT '',
 UNIQUE(team_id,project_id,task_id,id),
 UNIQUE(team_id,project_id,task_id,version),
 UNIQUE(team_id,project_id,task_id,client_request_id),
 FOREIGN KEY(team_id,project_id,task_id,submission_id) REFERENCES task_submissions(team_id,project_id,task_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,author_id) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT
);
CREATE INDEX task_submission_versions_task_idx ON task_submission_versions(team_id,project_id,task_id,version DESC);
CREATE TABLE task_reviews (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 task_id uuid NOT NULL,
 submission_version_id uuid NOT NULL,
 reviewer_id uuid NOT NULL,
 decision text NOT NULL CHECK(decision IN ('approve','reject')),
 comment text NOT NULL CHECK(length(btrim(comment)) BETWEEN 1 AND 4000),
 task_version integer NOT NULL CHECK(task_version>0),
 reviewed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 request_id text NOT NULL DEFAULT '',
 UNIQUE(team_id,project_id,task_id,submission_version_id),
 UNIQUE(team_id,project_id,task_id,submission_version_id,id),
 FOREIGN KEY(team_id,project_id,task_id,submission_version_id) REFERENCES task_submission_versions(team_id,project_id,task_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,reviewer_id) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT
);
CREATE INDEX task_reviews_reviewer_idx ON task_reviews(team_id,reviewer_id,reviewed_at);
CREATE TABLE submission_attachments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 task_id uuid NOT NULL,
 uploader_id uuid NOT NULL,
 client_request_id uuid NOT NULL,
 submission_version_id uuid,
 storage_key text NOT NULL UNIQUE CHECK(storage_key ~ '^[0-9a-f-]{36}/[0-9a-f-]{36}\.blob$' AND split_part(storage_key,'/',1)=team_id::text),
 file_name text NOT NULL CHECK(length(file_name) BETWEEN 1 AND 180),
 media_type text NOT NULL CHECK(media_type IN ('text/plain','application/pdf','image/png','image/jpeg')),
 byte_size bigint NOT NULL CHECK(byte_size BETWEEN 1 AND 10485760),
 sha256 text NOT NULL CHECK(sha256 ~ '^[0-9a-f]{64}$'),
 status text NOT NULL DEFAULT 'staged' CHECK(status IN ('staged','bound','expired')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 expires_at timestamptz NOT NULL,
 removed_at timestamptz,
 UNIQUE(team_id,project_id,task_id,id),
 UNIQUE(team_id,project_id,task_id,uploader_id,client_request_id),
 FOREIGN KEY(team_id,project_id,task_id) REFERENCES tasks(team_id,project_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,task_id,submission_version_id) REFERENCES task_submission_versions(team_id,project_id,task_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,uploader_id) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT,
 CHECK(expires_at>created_at),
 CHECK((status='bound' AND submission_version_id IS NOT NULL AND removed_at IS NULL) OR (status='staged' AND submission_version_id IS NULL AND removed_at IS NULL) OR (status='expired' AND submission_version_id IS NULL))
);
CREATE INDEX submission_attachments_version_idx ON submission_attachments(team_id,project_id,task_id,submission_version_id);
CREATE INDEX submission_attachments_quota_idx ON submission_attachments(team_id,status,expires_at) WHERE removed_at IS NULL;

ALTER TABLE tasks ADD COLUMN current_submission_version_id uuid;
ALTER TABLE tasks ADD COLUMN accepted_review_id uuid;
ALTER TABLE tasks ADD CONSTRAINT tasks_current_submission_fk FOREIGN KEY(team_id,project_id,id,current_submission_version_id) REFERENCES task_submission_versions(team_id,project_id,task_id,id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE tasks ADD CONSTRAINT tasks_accepted_review_fk FOREIGN KEY(team_id,project_id,id,current_submission_version_id,accepted_review_id) REFERENCES task_reviews(team_id,project_id,task_id,submission_version_id,id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;
CREATE VIEW task_acceptance_evidence AS
 SELECT t.team_id,t.project_id,t.id AS task_id,t.current_submission_version_id AS current_version_id,r.submission_version_id AS reviewed_version_id,r.id AS review_id,r.decision
 FROM tasks t JOIN task_reviews r ON r.team_id=t.team_id AND r.project_id=t.project_id AND r.task_id=t.id AND r.submission_version_id=t.current_submission_version_id AND r.id=t.accepted_review_id
 WHERE t.status='completed' AND r.decision='approve';
CREATE TRIGGER task_submissions_no_mutation BEFORE UPDATE OR DELETE ON task_submissions FOR EACH ROW EXECUTE FUNCTION audit_events_forbid_mutation();
CREATE TRIGGER task_submission_versions_no_mutation BEFORE UPDATE OR DELETE ON task_submission_versions FOR EACH ROW EXECUTE FUNCTION audit_events_forbid_mutation();
CREATE TRIGGER task_reviews_no_mutation BEFORE UPDATE OR DELETE ON task_reviews FOR EACH ROW EXECUTE FUNCTION audit_events_forbid_mutation();

-- +goose StatementBegin
CREATE FUNCTION phase04_guard_attachment() RETURNS trigger AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'attachment metadata cannot be deleted' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.status='bound' THEN RAISE EXCEPTION 'bound attachments are immutable' USING ERRCODE='23514'; END IF;
  IF (to_jsonb(NEW)-ARRAY['status','submission_version_id','removed_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['status','submission_version_id','removed_at']) THEN RAISE EXCEPTION 'attachment metadata is immutable' USING ERRCODE='23514'; END IF;
  IF OLD.status='expired' AND (NEW.status<>'expired' OR OLD.removed_at IS NOT NULL) THEN RAISE EXCEPTION 'expired attachment cannot be reused' USING ERRCODE='23514'; END IF;
 END IF;
 IF NEW.status='bound' AND NOT EXISTS(SELECT 1 FROM task_submission_versions v WHERE v.team_id=NEW.team_id AND v.project_id=NEW.project_id AND v.task_id=NEW.task_id AND v.id=NEW.submission_version_id AND v.author_id=NEW.uploader_id AND v.creation_xid=pg_current_xact_id()) THEN RAISE EXCEPTION 'attachment must bind to its author version in the creating transaction' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER submission_attachments_guard BEFORE INSERT OR UPDATE OR DELETE ON submission_attachments FOR EACH ROW EXECUTE FUNCTION phase04_guard_attachment();

-- +goose StatementBegin
CREATE FUNCTION phase04_guard_version_insert() RETURNS trigger AS $$
DECLARE t tasks%ROWTYPE;
BEGIN
 IF NEW.creation_xid<>pg_current_xact_id() THEN RAISE EXCEPTION 'submission transaction marker must be current' USING ERRCODE='23514'; END IF;
 SELECT * INTO t FROM tasks WHERE team_id=NEW.team_id AND project_id=NEW.project_id AND id=NEW.task_id FOR UPDATE;
 IF NOT FOUND OR t.status NOT IN ('in_progress','rejected') OR t.version<>NEW.based_on_task_version THEN RAISE EXCEPTION 'submission requires current execution state' USING ERRCODE='23514'; END IF;
 IF EXISTS(SELECT 1 FROM tasks WHERE team_id=t.team_id AND project_id=t.project_id AND parent_id=t.id) THEN RAISE EXCEPTION 'only leaves accept submissions' USING ERRCODE='23514'; END IF;
 IF NOT EXISTS(SELECT 1 FROM projects WHERE team_id=t.team_id AND id=t.project_id AND status='active') THEN RAISE EXCEPTION 'archived project' USING ERRCODE='23514'; END IF;
 IF NOT EXISTS(SELECT 1 FROM project_members pm JOIN team_members tm ON tm.team_id=pm.team_id AND tm.user_id=pm.user_id WHERE pm.team_id=t.team_id AND pm.project_id=t.project_id AND pm.user_id=NEW.author_id AND pm.status='active' AND tm.status='active' AND (t.owner_id=NEW.author_id OR EXISTS(SELECT 1 FROM task_collaborators c WHERE c.team_id=t.team_id AND c.project_id=t.project_id AND c.task_id=t.id AND c.user_id=NEW.author_id))) THEN RAISE EXCEPTION 'submission author is not authorized' USING ERRCODE='23514'; END IF;
 IF NEW.version<>COALESCE((SELECT max(version)+1 FROM task_submission_versions WHERE team_id=t.team_id AND project_id=t.project_id AND task_id=t.id),1) THEN RAISE EXCEPTION 'submission version must increase monotonically' USING ERRCODE='23514'; END IF;
 IF EXISTS(SELECT 1 FROM task_dependencies d WHERE d.team_id=t.team_id AND d.project_id=t.project_id AND d.task_id=t.id AND d.ignored_at IS NULL AND NOT EXISTS(SELECT 1 FROM task_acceptance_evidence e WHERE e.team_id=d.team_id AND e.project_id=d.project_id AND e.task_id=d.predecessor_id)) THEN RAISE EXCEPTION 'blocked task cannot submit' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER task_submission_version_insert_guard BEFORE INSERT ON task_submission_versions FOR EACH ROW EXECUTE FUNCTION phase04_guard_version_insert();

-- +goose StatementBegin
CREATE FUNCTION phase04_guard_version_commit() RETURNS trigger AS $$
DECLARE t tasks%ROWTYPE; file_count bigint; file_bytes bigint;
BEGIN
 SELECT * INTO t FROM tasks WHERE team_id=NEW.team_id AND project_id=NEW.project_id AND id=NEW.task_id FOR UPDATE;
 IF t.current_submission_version_id IS DISTINCT FROM NEW.id OR t.status<>'awaiting_review' OR t.version<>NEW.based_on_task_version+1 THEN RAISE EXCEPTION 'submission and task must commit together' USING ERRCODE='23514'; END IF;
 SELECT count(*),COALESCE(sum(byte_size),0) INTO file_count,file_bytes FROM submission_attachments WHERE team_id=NEW.team_id AND project_id=NEW.project_id AND task_id=NEW.task_id AND submission_version_id=NEW.id AND status='bound';
 IF file_count>5 OR file_bytes>26214400 OR (length(btrim(NEW.summary))=0 AND jsonb_array_length(NEW.evidence_references)=0 AND file_count=0) THEN RAISE EXCEPTION 'empty or oversized submission' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER task_submission_version_commit_guard AFTER INSERT ON task_submission_versions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION phase04_guard_version_commit();

-- +goose StatementBegin
CREATE FUNCTION phase04_guard_review_insert() RETURNS trigger AS $$
DECLARE t tasks%ROWTYPE; author uuid;
BEGIN
 SELECT * INTO t FROM tasks WHERE team_id=NEW.team_id AND project_id=NEW.project_id AND id=NEW.task_id FOR UPDATE;
 SELECT author_id INTO author FROM task_submission_versions WHERE team_id=NEW.team_id AND project_id=NEW.project_id AND task_id=NEW.task_id AND id=NEW.submission_version_id;
 IF t.status<>'awaiting_review' OR t.current_submission_version_id IS DISTINCT FROM NEW.submission_version_id OR t.version<>NEW.task_version THEN RAISE EXCEPTION 'review must target current awaiting version' USING ERRCODE='23514'; END IF;
 IF author=NEW.reviewer_id OR NOT phase04_can_review(NEW.team_id,NEW.project_id,NEW.reviewer_id) THEN RAISE EXCEPTION 'reviewer unauthorized or self review' USING ERRCODE='23514'; END IF;
 IF NOT EXISTS(SELECT 1 FROM projects WHERE team_id=t.team_id AND id=t.project_id AND status='active') OR EXISTS(SELECT 1 FROM tasks WHERE team_id=t.team_id AND project_id=t.project_id AND parent_id=t.id) THEN RAISE EXCEPTION 'only active project leaves can be reviewed' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER task_review_insert_guard BEFORE INSERT ON task_reviews FOR EACH ROW EXECUTE FUNCTION phase04_guard_review_insert();

-- +goose StatementBegin
CREATE FUNCTION phase04_guard_review_commit() RETURNS trigger AS $$
DECLARE t tasks%ROWTYPE;
BEGIN
 SELECT * INTO t FROM tasks WHERE team_id=NEW.team_id AND project_id=NEW.project_id AND id=NEW.task_id FOR UPDATE;
 IF t.current_submission_version_id IS DISTINCT FROM NEW.submission_version_id OR t.version<>NEW.task_version+1 OR (NEW.decision='approve' AND (t.status<>'completed' OR t.accepted_review_id IS DISTINCT FROM NEW.id)) OR (NEW.decision='reject' AND (t.status<>'rejected' OR t.accepted_review_id IS NOT NULL)) THEN RAISE EXCEPTION 'review and task must commit together' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER task_review_commit_guard AFTER INSERT ON task_reviews DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION phase04_guard_review_commit();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION tasks_guard_execution() RETURNS trigger AS $$
BEGIN
 IF TG_OP='INSERT' AND NEW.status<>'pending' THEN RAISE EXCEPTION 'new tasks must start pending' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.status='completed' AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'completed task is immutable' USING ERRCODE='23514'; END IF;
  IF OLD.status='cancelled' AND (NEW.status<>OLD.status OR NEW.actual_progress<>OLD.actual_progress) THEN RAISE EXCEPTION 'cancelled task is terminal' USING ERRCODE='23514'; END IF;
  IF NEW.status IS DISTINCT FROM OLD.status AND NOT ((OLD.status='pending' AND NEW.status IN ('in_progress','cancelled')) OR (OLD.status='in_progress' AND NEW.status IN ('awaiting_review','cancelled')) OR (OLD.status='awaiting_review' AND NEW.status IN ('completed','rejected')) OR (OLD.status='rejected' AND NEW.status IN ('in_progress','awaiting_review','cancelled'))) THEN RAISE EXCEPTION 'illegal task transition' USING ERRCODE='23514'; END IF;
  IF NEW.status IS DISTINCT FROM OLD.status AND NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'task transition requires version increment' USING ERRCODE='23514'; END IF;
  IF NEW.status='awaiting_review' AND NEW.status IS DISTINCT FROM OLD.status AND (NEW.current_submission_version_id IS NULL OR NEW.current_submission_version_id IS NOT DISTINCT FROM OLD.current_submission_version_id) THEN RAISE EXCEPTION 'a new submission version is required' USING ERRCODE='23514'; END IF;
  IF NEW.status=OLD.status AND (NEW.current_submission_version_id IS DISTINCT FROM OLD.current_submission_version_id OR NEW.accepted_review_id IS DISTINCT FROM OLD.accepted_review_id) THEN RAISE EXCEPTION 'evidence cannot be rebound without a transition' USING ERRCODE='23514'; END IF;
 END IF;
 IF NEW.status<>'completed' AND NEW.accepted_review_id IS NOT NULL THEN RAISE EXCEPTION 'approval reference requires completed' USING ERRCODE='23514'; END IF;
 IF NOT EXISTS(SELECT 1 FROM project_members pm JOIN team_members tm ON tm.team_id=pm.team_id AND tm.user_id=pm.user_id WHERE pm.team_id=NEW.team_id AND pm.project_id=NEW.project_id AND pm.user_id=NEW.owner_id AND pm.status='active' AND tm.status='active') THEN
  -- Historical authors can leave. A qualified independent review may still
  -- close their immutable evidence; deferred guards require the actual review.
  -- This exception cannot edit assignment, content, structure, weight or proof.
  IF TG_OP='UPDATE' AND OLD.status='awaiting_review' AND NEW.status IN ('completed','rejected') AND (to_jsonb(NEW)-ARRAY['status','accepted_review_id','version','updated_at']) IS NOT DISTINCT FROM (to_jsonb(OLD)-ARRAY['status','accepted_review_id','version','updated_at']) THEN RETURN NEW; END IF;
  RAISE EXCEPTION 'task owner requires active project and team membership' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION phase04_guard_task_evidence() RETURNS trigger AS $$
DECLARE t tasks%ROWTYPE; latest uuid;
BEGIN
 SELECT * INTO t FROM tasks WHERE team_id=NEW.team_id AND project_id=NEW.project_id AND id=NEW.id;
 IF t.current_submission_version_id IS NOT NULL THEN
  SELECT id INTO latest FROM task_submission_versions WHERE team_id=t.team_id AND project_id=t.project_id AND task_id=t.id ORDER BY version DESC LIMIT 1;
  IF latest IS DISTINCT FROM t.current_submission_version_id THEN RAISE EXCEPTION 'task must reference its latest version' USING ERRCODE='23514'; END IF;
 END IF;
 IF t.status IN ('awaiting_review','completed','rejected') THEN
  IF t.current_submission_version_id IS NULL OR EXISTS(SELECT 1 FROM tasks WHERE team_id=t.team_id AND project_id=t.project_id AND parent_id=t.id) THEN RAISE EXCEPTION 'version-bound leaf evidence is required' USING ERRCODE='23514'; END IF;
  IF t.status='completed' AND NOT EXISTS(SELECT 1 FROM task_acceptance_evidence e WHERE e.team_id=t.team_id AND e.project_id=t.project_id AND e.task_id=t.id) THEN RAISE EXCEPTION 'current version approval is required' USING ERRCODE='23514'; END IF;
  IF t.status='rejected' AND NOT EXISTS(SELECT 1 FROM task_reviews r WHERE r.team_id=t.team_id AND r.project_id=t.project_id AND r.task_id=t.id AND r.submission_version_id=t.current_submission_version_id AND r.decision='reject') THEN RAISE EXCEPTION 'current version rejection is required' USING ERRCODE='23514'; END IF;
  IF t.status='awaiting_review' AND EXISTS(SELECT 1 FROM task_reviews r WHERE r.team_id=t.team_id AND r.project_id=t.project_id AND r.task_id=t.id AND r.submission_version_id=t.current_submission_version_id) THEN RAISE EXCEPTION 'review decision must transition the task' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NULL;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER tasks_evidence_guard AFTER INSERT OR UPDATE ON tasks DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION phase04_guard_task_evidence();

-- Completed assignments are historical, not outstanding work that blocks exit.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION tasks_guard_project_departure() RETURNS trigger AS $$
BEGIN
 IF OLD.status='active' AND NEW.status='removed' AND EXISTS(SELECT 1 FROM team_members WHERE team_id=OLD.team_id AND user_id=OLD.user_id AND status='active') THEN
  IF EXISTS(SELECT 1 FROM tasks t WHERE t.team_id=OLD.team_id AND t.project_id=OLD.project_id AND t.status NOT IN ('cancelled','completed') AND (t.owner_id=OLD.user_id OR EXISTS(SELECT 1 FROM task_collaborators c WHERE c.team_id=t.team_id AND c.project_id=t.project_id AND c.task_id=t.id AND c.user_id=OLD.user_id))) THEN RAISE EXCEPTION 'reassign tasks before removing project member' USING ERRCODE='23514',CONSTRAINT='project_member_has_tasks'; END IF;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
-- Fail closed rather than silently deleting immutable production evidence.
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM task_submissions) OR EXISTS(SELECT 1 FROM submission_attachments) THEN RAISE EXCEPTION 'cannot roll back Phase 04 with business evidence; restore a verified backup instead'; END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER tasks_evidence_guard ON tasks;
DROP FUNCTION phase04_guard_task_evidence();
ALTER TABLE tasks DROP CONSTRAINT tasks_accepted_review_fk;
ALTER TABLE tasks DROP CONSTRAINT tasks_current_submission_fk;
DROP VIEW task_acceptance_evidence;
ALTER TABLE tasks DROP COLUMN accepted_review_id;
ALTER TABLE tasks DROP COLUMN current_submission_version_id;
DROP TABLE submission_attachments;
DROP FUNCTION phase04_guard_attachment();
DROP TABLE task_reviews;
DROP FUNCTION phase04_guard_review_insert();
DROP FUNCTION phase04_guard_review_commit();
DROP TABLE task_submission_versions;
DROP FUNCTION phase04_guard_version_insert();
DROP FUNCTION phase04_guard_version_commit();
DROP TABLE task_submissions;
DROP FUNCTION phase04_can_review(uuid,uuid,uuid);
DROP TABLE project_reviewers;
DROP TRIGGER project_members_authorization_guard ON project_members;
DROP FUNCTION project_members_bump_authorization();
ALTER TABLE project_members DROP COLUMN authorization_version;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION tasks_guard_execution() RETURNS trigger AS $$
BEGIN
 IF NEW.status IN ('awaiting_review','completed','rejected') THEN RAISE EXCEPTION 'Phase 04 evidence is required' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' AND OLD.status='cancelled' AND (NEW.status<>OLD.status OR NEW.actual_progress<>OLD.actual_progress) THEN RAISE EXCEPTION 'cancelled task is terminal' USING ERRCODE='23514'; END IF;
 IF NOT EXISTS(SELECT 1 FROM project_members pm JOIN team_members tm ON tm.team_id=pm.team_id AND tm.user_id=pm.user_id WHERE pm.team_id=NEW.team_id AND pm.project_id=NEW.project_id AND pm.user_id=NEW.owner_id AND pm.status='active' AND tm.status='active') THEN RAISE EXCEPTION 'task owner requires active project and team membership' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION tasks_guard_project_departure() RETURNS trigger AS $$
BEGIN
 IF OLD.status='active' AND NEW.status='removed' AND EXISTS(SELECT 1 FROM team_members WHERE team_id=OLD.team_id AND user_id=OLD.user_id AND status='active') THEN
  IF EXISTS(SELECT 1 FROM tasks t WHERE t.team_id=OLD.team_id AND t.project_id=OLD.project_id AND t.status<>'cancelled' AND (t.owner_id=OLD.user_id OR EXISTS(SELECT 1 FROM task_collaborators c WHERE c.team_id=t.team_id AND c.project_id=t.project_id AND c.task_id=t.id AND c.user_id=OLD.user_id))) THEN RAISE EXCEPTION 'reassign tasks before removing project member' USING ERRCODE='23514',CONSTRAINT='project_member_has_tasks'; END IF;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
-- +goose StatementEnd
