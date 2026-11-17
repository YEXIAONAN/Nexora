-- +goose Up
-- Phase 02 team tenancy: teams, per-team memberships with independent roles,
-- and controlled invitations. All team resources hang off teams.id.

CREATE TABLE teams (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text        NOT NULL,
    description text        NOT NULL DEFAULT '',
    created_by  uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    CONSTRAINT teams_name_length CHECK (length(btrim(name)) BETWEEN 1 AND 64),
    CONSTRAINT teams_description_length CHECK (length(description) <= 500)
);

CREATE TABLE team_members (
    team_id    uuid        NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    role       text        NOT NULL,
    status     text        NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    removed_at timestamptz,
    PRIMARY KEY (team_id, user_id),
    CONSTRAINT team_members_role_known CHECK (role IN ('owner', 'admin', 'member')),
    CONSTRAINT team_members_status_known CHECK (status IN ('active', 'removed')),
    CONSTRAINT team_members_removal_consistency CHECK (
        (status = 'active' AND removed_at IS NULL)
        OR (status = 'removed' AND removed_at IS NOT NULL)
    )
);

CREATE INDEX team_members_user_status_idx ON team_members (user_id, status);
CREATE INDEX team_members_active_owners_idx ON team_members (team_id)
    WHERE status = 'active' AND role = 'owner';

-- The service serializes member mutations with SELECT ... FOR UPDATE on the
-- teams row, so this count is stable under concurrent role/removal changes.
-- +goose StatementBegin
CREATE FUNCTION team_members_guard_last_owner() RETURNS trigger AS $$
DECLARE
    remaining integer;
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.status = 'active' AND OLD.role = 'owner' THEN
            SELECT count(*) INTO remaining FROM team_members
            WHERE team_id = OLD.team_id AND status = 'active' AND role = 'owner'
              AND user_id <> OLD.user_id;
            IF remaining = 0 THEN
                RAISE EXCEPTION 'team must retain at least one active owner';
            END IF;
        END IF;
        RETURN OLD;
    END IF;

    IF OLD.status = 'active' AND OLD.role = 'owner'
       AND (NEW.status <> 'active' OR NEW.role <> 'owner') THEN
        SELECT count(*) INTO remaining FROM team_members
        WHERE team_id = OLD.team_id AND status = 'active' AND role = 'owner'
          AND user_id <> OLD.user_id;
        IF remaining = 0 THEN
            RAISE EXCEPTION 'team must retain at least one active owner';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER team_members_last_owner_guard
    BEFORE UPDATE OF role, status OR DELETE ON team_members
    FOR EACH ROW EXECUTE FUNCTION team_members_guard_last_owner();

CREATE TABLE team_invitations (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id         uuid        NOT NULL REFERENCES teams (id) ON DELETE RESTRICT,
    invited_user_id uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    invited_by      uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    role            text        NOT NULL,
    status          text        NOT NULL DEFAULT 'pending',
    expires_at      timestamptz NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    responded_at    timestamptz,
    CONSTRAINT team_invitations_role_known CHECK (role IN ('admin', 'member')),
    CONSTRAINT team_invitations_status_known CHECK (status IN ('pending', 'accepted', 'declined', 'cancelled')),
    CONSTRAINT team_invitations_expiry_after_creation CHECK (expires_at > created_at),
    CONSTRAINT team_invitations_response_consistency CHECK (
        (status = 'pending' AND responded_at IS NULL)
        OR (status <> 'pending' AND responded_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX team_invitations_one_pending ON team_invitations (team_id, invited_user_id)
    WHERE status = 'pending';
CREATE INDEX team_invitations_invitee_status_idx ON team_invitations (invited_user_id, status);
CREATE INDEX team_invitations_team_status_idx ON team_invitations (team_id, status);

ALTER TABLE audit_events
    ADD CONSTRAINT audit_events_team_fk FOREIGN KEY (team_id) REFERENCES teams (id) ON DELETE RESTRICT;

COMMENT ON TABLE teams IS 'Tenant boundary; every team-owned resource references teams.id.';
COMMENT ON TABLE team_members IS 'Per-team roles; composite PK (team_id, user_id) is the target for future composite FKs.';
COMMENT ON TABLE team_invitations IS 'Controlled join channel: authorized inviter, existing account, expiry, invitee-only response.';

-- +goose Down
ALTER TABLE audit_events DROP CONSTRAINT audit_events_team_fk;
DROP TABLE team_invitations;
DROP TRIGGER IF EXISTS team_members_last_owner_guard ON team_members;
DROP FUNCTION IF EXISTS team_members_guard_last_owner();
DROP TABLE team_members;
DROP TABLE teams;