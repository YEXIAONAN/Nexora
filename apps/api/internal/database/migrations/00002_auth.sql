-- +goose Up
-- Phase 02 identity foundation: accounts, server-side sessions, platform
-- roles and the append-only audit log. Team tenancy arrives in 00003.

CREATE TABLE users (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    username      text        NOT NULL UNIQUE,
    display_name  text        NOT NULL DEFAULT '',
    password_hash text        NOT NULL,
    system_root   boolean     NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_username_format CHECK (username ~ '^[a-z0-9_-]{3,32}$'),
    CONSTRAINT users_display_name_length CHECK (length(display_name) <= 64),
    CONSTRAINT users_password_hash_not_blank CHECK (length(btrim(password_hash)) > 0)
);

-- Exactly one ROOT identity per installation, enforced in the database so
-- concurrent bootstrap attempts cannot create two ROOTs.
CREATE UNIQUE INDEX users_single_system_root ON users (system_root) WHERE system_root;

CREATE TABLE sessions (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    token_hash text        NOT NULL UNIQUE,
    user_agent text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    CONSTRAINT sessions_token_hash_format CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT sessions_user_agent_length CHECK (length(user_agent) <= 256),
    CONSTRAINT sessions_expiry_after_creation CHECK (expires_at > created_at),
    CONSTRAINT sessions_revocation_after_creation CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE platform_roles (
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    role       text        NOT NULL,
    granted_by uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role),
    CONSTRAINT platform_roles_role_known CHECK (role IN ('platform_admin'))
);

CREATE TABLE audit_events (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id    uuid        REFERENCES users (id) ON DELETE RESTRICT,
    action      text        NOT NULL,
    target_type text,
    target_id   uuid,
    team_id     uuid,
    request_id  text        NOT NULL DEFAULT '',
    details     jsonb       NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT audit_events_action_not_blank CHECK (length(btrim(action)) > 0)
);

CREATE INDEX audit_events_team_created_idx ON audit_events (team_id, created_at);
CREATE INDEX audit_events_actor_created_idx ON audit_events (actor_id, created_at);
CREATE INDEX audit_events_target_idx ON audit_events (target_type, target_id);
CREATE INDEX audit_events_created_idx ON audit_events (created_at);

-- +goose StatementBegin
CREATE FUNCTION audit_events_forbid_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER audit_events_no_mutation
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION audit_events_forbid_mutation();

COMMENT ON TABLE users IS 'Accounts; system_root marks the single immutable ROOT identity.';
COMMENT ON TABLE sessions IS 'Server-side sessions storing only SHA-256 token digests.';
COMMENT ON TABLE platform_roles IS 'Platform roles granted by ROOT; ROOT itself never appears here.';
COMMENT ON TABLE audit_events IS 'Append-only operation audit written inside business transactions.';

-- +goose Down
DROP TRIGGER IF EXISTS audit_events_no_mutation ON audit_events;
DROP FUNCTION IF EXISTS audit_events_forbid_mutation();
DROP TABLE audit_events;
DROP TABLE platform_roles;
DROP TABLE sessions;
DROP TABLE users;