-- +goose Up
CREATE TABLE tasks (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 parent_id uuid,
 depth smallint NOT NULL CHECK(depth BETWEEN 1 AND 3),
 title text NOT NULL CHECK(length(btrim(title)) BETWEEN 1 AND 200),
 description text NOT NULL DEFAULT '' CHECK(length(description)<=8000),
 owner_id uuid NOT NULL,
 priority text NOT NULL DEFAULT 'normal' CHECK(priority IN ('low','normal','high','urgent')),
 planned_start_at timestamptz,
 planned_end_at timestamptz,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','in_progress','awaiting_review','completed','rejected','cancelled')),
 actual_progress smallint NOT NULL DEFAULT 0 CHECK(actual_progress BETWEEN 0 AND 99),
 weight integer NOT NULL CHECK(weight BETWEEN 1 AND 1000000),
 cancellation_reason text NOT NULL DEFAULT '' CHECK(length(cancellation_reason)<=1000),
 version integer NOT NULL DEFAULT 1 CHECK(version>0),
 created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(team_id,project_id,id),
 FOREIGN KEY(team_id,project_id) REFERENCES projects(team_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,parent_id) REFERENCES tasks(team_id,project_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,owner_id) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT,
 CHECK(parent_id IS NULL OR parent_id<>id),
 CHECK((parent_id IS NULL AND depth=1) OR (parent_id IS NOT NULL AND depth>1)),
 CHECK(planned_start_at IS NULL OR planned_end_at IS NULL OR planned_start_at<=planned_end_at),
 CHECK(status<>'pending' OR actual_progress=0),
 CHECK((status='cancelled' AND length(btrim(cancellation_reason))>0) OR (status<>'cancelled' AND cancellation_reason=''))
);
CREATE INDEX tasks_tree_idx ON tasks(team_id,project_id,parent_id,created_at,id);
CREATE INDEX tasks_owner_idx ON tasks(team_id,project_id,owner_id);
CREATE TABLE task_collaborators (
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 task_id uuid NOT NULL,
 user_id uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(team_id,project_id,task_id,user_id),
 FOREIGN KEY(team_id,project_id,task_id) REFERENCES tasks(team_id,project_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,user_id) REFERENCES project_members(team_id,project_id,user_id) ON DELETE RESTRICT
);
CREATE INDEX task_collaborators_user_idx ON task_collaborators(team_id,project_id,user_id);
CREATE TABLE task_dependencies (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 task_id uuid NOT NULL,
 predecessor_id uuid NOT NULL,
 created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 created_at timestamptz NOT NULL DEFAULT now(),
 ignored_by uuid REFERENCES users(id) ON DELETE RESTRICT,
 ignored_at timestamptz,
 ignore_reason text NOT NULL DEFAULT '' CHECK(length(ignore_reason)<=1000),
 UNIQUE(team_id,project_id,task_id,predecessor_id),
 FOREIGN KEY(team_id,project_id,task_id) REFERENCES tasks(team_id,project_id,id) ON DELETE RESTRICT,
 FOREIGN KEY(team_id,project_id,predecessor_id) REFERENCES tasks(team_id,project_id,id) ON DELETE RESTRICT,
 CHECK(task_id<>predecessor_id),
 CHECK((ignored_by IS NULL AND ignored_at IS NULL AND ignore_reason='') OR (ignored_by IS NOT NULL AND ignored_at IS NOT NULL AND length(btrim(ignore_reason))>0))
);
CREATE INDEX task_dependencies_predecessor_idx ON task_dependencies(team_id,project_id,predecessor_id);
CREATE TABLE task_progress_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 task_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 event_type text NOT NULL,
 old_status text NOT NULL,
 new_status text NOT NULL,
 old_progress smallint NOT NULL,
 new_progress smallint NOT NULL,
 old_weight integer NOT NULL,
 new_weight integer NOT NULL,
 reason text NOT NULL DEFAULT '',
 before_project_bps integer NOT NULL CHECK(before_project_bps BETWEEN 0 AND 10000),
 after_project_bps integer NOT NULL CHECK(after_project_bps BETWEEN 0 AND 10000),
 request_id text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(team_id,project_id,task_id) REFERENCES tasks(team_id,project_id,id) ON DELETE RESTRICT
);
CREATE INDEX task_progress_history_task_idx ON task_progress_history(team_id,project_id,task_id,id DESC);
CREATE TABLE project_progress_snapshots (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 team_id uuid NOT NULL,
 project_id uuid NOT NULL,
 task_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 event_type text NOT NULL,
 progress_bps integer NOT NULL CHECK(progress_bps BETWEEN 0 AND 10000),
 weighted_sum numeric NOT NULL CHECK(weighted_sum>=0),
 total_weight numeric NOT NULL CHECK(total_weight>=0),
 leaf_count integer NOT NULL CHECK(leaf_count>=0),
 request_id text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(team_id,project_id,task_id) REFERENCES tasks(team_id,project_id,id) ON DELETE RESTRICT
);
CREATE INDEX project_progress_snapshots_project_idx ON project_progress_snapshots(team_id,project_id,id DESC);
CREATE TRIGGER task_progress_history_no_mutation BEFORE UPDATE OR DELETE ON task_progress_history FOR EACH ROW EXECUTE FUNCTION audit_events_forbid_mutation();
CREATE TRIGGER project_progress_snapshots_no_mutation BEFORE UPDATE OR DELETE ON project_progress_snapshots FOR EACH ROW EXECUTE FUNCTION audit_events_forbid_mutation();

-- Secondary integrity guard. The service locks the team before all writes and
-- computes the entire subtree. A deferred trigger verifies the final tree, so
-- subtree depth updates need not pass through inconsistent intermediate rows.
-- +goose StatementBegin
CREATE FUNCTION tasks_guard_structure() RETURNS trigger AS $$
BEGIN
 IF EXISTS (
  SELECT 1 FROM tasks t LEFT JOIN tasks p ON p.team_id=t.team_id AND p.project_id=t.project_id AND p.id=t.parent_id
  WHERE t.team_id=NEW.team_id AND t.project_id=NEW.project_id
    AND ((t.parent_id IS NOT NULL AND (p.id IS NULL OR t.depth<>p.depth+1))
      OR (t.parent_id IS NULL AND t.depth<>1))
 ) THEN RAISE EXCEPTION 'invalid task tree depth' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER tasks_structure_guard AFTER INSERT OR UPDATE ON tasks DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION tasks_guard_structure();

-- +goose StatementBegin
CREATE FUNCTION tasks_guard_execution() RETURNS trigger AS $$
BEGIN
 IF NEW.status IN ('awaiting_review','completed','rejected') THEN
  RAISE EXCEPTION 'Phase 04 evidence is required' USING ERRCODE='23514';
 END IF;
 IF TG_OP='UPDATE' AND OLD.status='cancelled' AND (NEW.status<>OLD.status OR NEW.actual_progress<>OLD.actual_progress) THEN
  RAISE EXCEPTION 'cancelled task is terminal' USING ERRCODE='23514';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM project_members pm JOIN team_members tm ON tm.team_id=pm.team_id AND tm.user_id=pm.user_id WHERE pm.team_id=NEW.team_id AND pm.project_id=NEW.project_id AND pm.user_id=NEW.owner_id AND pm.status='active' AND tm.status='active') THEN
  RAISE EXCEPTION 'task owner requires active project and team membership' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER tasks_execution_guard BEFORE INSERT OR UPDATE ON tasks FOR EACH ROW EXECUTE FUNCTION tasks_guard_execution();

-- +goose StatementBegin
CREATE FUNCTION tasks_guard_project_departure() RETURNS trigger AS $$
BEGIN
 IF OLD.status='active' AND NEW.status='removed' AND EXISTS(SELECT 1 FROM team_members WHERE team_id=OLD.team_id AND user_id=OLD.user_id AND status='active') THEN
  IF EXISTS(SELECT 1 FROM tasks t WHERE t.team_id=OLD.team_id AND t.project_id=OLD.project_id AND t.status<>'cancelled' AND (t.owner_id=OLD.user_id OR EXISTS(SELECT 1 FROM task_collaborators c WHERE c.team_id=t.team_id AND c.project_id=t.project_id AND c.task_id=t.id AND c.user_id=OLD.user_id))) THEN
   RAISE EXCEPTION 'reassign tasks before removing project member' USING ERRCODE='23514',CONSTRAINT='project_member_has_tasks';
  END IF;
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER project_member_tasks_guard BEFORE UPDATE OF status ON project_members FOR EACH ROW EXECUTE FUNCTION tasks_guard_project_departure();

-- +goose Down
DROP TRIGGER project_member_tasks_guard ON project_members;
DROP FUNCTION tasks_guard_project_departure();
DROP TRIGGER tasks_execution_guard ON tasks;
DROP FUNCTION tasks_guard_execution();
DROP TRIGGER tasks_structure_guard ON tasks;
DROP FUNCTION tasks_guard_structure();
DROP TABLE project_progress_snapshots;
DROP TABLE task_progress_history;
DROP TABLE task_dependencies;
DROP TABLE task_collaborators;
DROP TABLE tasks;
