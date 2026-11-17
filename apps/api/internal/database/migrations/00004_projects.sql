-- +goose Up
CREATE TABLE projects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id uuid NOT NULL REFERENCES teams(id) ON DELETE RESTRICT,
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 4000),
    lead_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
    planned_start_at timestamptz,
    planned_end_at timestamptz,
    archived_at timestamptz,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (team_id, id),
    FOREIGN KEY (team_id, lead_id) REFERENCES team_members(team_id,user_id) ON DELETE RESTRICT,
    CHECK (planned_start_at IS NULL OR planned_end_at IS NULL OR planned_start_at <= planned_end_at),
    CHECK ((status='active' AND archived_at IS NULL) OR (status='archived' AND archived_at IS NOT NULL))
);
CREATE INDEX projects_team_created_idx ON projects(team_id,created_at,id);
CREATE INDEX projects_lead_idx ON projects(team_id,lead_id);
CREATE TABLE project_members (
    team_id uuid NOT NULL,
    project_id uuid NOT NULL,
    user_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','removed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id,project_id,user_id),
    FOREIGN KEY (team_id,project_id) REFERENCES projects(team_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (team_id,user_id) REFERENCES team_members(team_id,user_id) ON DELETE RESTRICT
);
CREATE INDEX project_members_user_idx ON project_members(team_id,user_id,status);
CREATE TABLE project_milestones (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id uuid NOT NULL,
    project_id uuid NOT NULL,
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 4000),
    planned_at timestamptz,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (team_id,project_id) REFERENCES projects(team_id,id) ON DELETE RESTRICT
);
CREATE INDEX project_milestones_project_idx ON project_milestones(team_id,project_id,planned_at,id);

-- Active membership is a cross-table condition, not a CHECK. Services acquire
-- the team lock before authorization or qualification changes.
-- +goose StatementBegin
CREATE FUNCTION projects_guard_lead() RETURNS trigger AS $$
BEGIN
    IF TG_OP='UPDATE' AND OLD.status='archived' THEN
        RAISE EXCEPTION 'archived project is read only' USING ERRCODE='23514';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM team_members WHERE team_id=NEW.team_id AND user_id=NEW.lead_id AND status='active') THEN
        RAISE EXCEPTION 'project lead requires active team membership' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER projects_lead_guard BEFORE INSERT OR UPDATE ON projects FOR EACH ROW EXECUTE FUNCTION projects_guard_lead();

-- +goose StatementBegin
CREATE FUNCTION projects_guard_team_departure() RETURNS trigger AS $$
BEGIN
    IF OLD.status='active' AND (TG_OP='DELETE' OR NEW.status<>'active') THEN
        IF EXISTS (SELECT 1 FROM projects WHERE team_id=OLD.team_id AND lead_id=OLD.user_id) THEN
            RAISE EXCEPTION 'project lead must be handed off before leaving team' USING ERRCODE='23514', CONSTRAINT='project_lead_handoff_required';
        END IF;
        UPDATE project_members SET status='removed',updated_at=now()
        WHERE team_id=OLD.team_id AND user_id=OLD.user_id AND status='active';
    END IF;
    IF TG_OP='DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER project_lead_team_departure_guard BEFORE UPDATE OF status OR DELETE ON team_members FOR EACH ROW EXECUTE FUNCTION projects_guard_team_departure();

-- +goose Down
DROP TRIGGER project_lead_team_departure_guard ON team_members;
DROP FUNCTION projects_guard_team_departure();
DROP TRIGGER projects_lead_guard ON projects;
DROP FUNCTION projects_guard_lead();
DROP TABLE project_milestones;
DROP TABLE project_members;
DROP TABLE projects;
