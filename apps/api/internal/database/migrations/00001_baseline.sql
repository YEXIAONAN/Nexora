-- +goose Up
-- Phase 0 baseline: platform-level settings that are not owned by a team.
-- The installation identifier gives this deployment a stable identity that is
-- safe to expose to authenticated clients and useful in diagnostics.
CREATE TABLE system_settings (
    key         text        PRIMARY KEY,
    value       jsonb       NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT system_settings_key_not_blank CHECK (length(btrim(key)) > 0)
);

COMMENT ON TABLE system_settings IS 'Platform-level key/value settings excluded from team tenancy.';

INSERT INTO system_settings (key, value)
VALUES ('installation_id', to_jsonb(gen_random_uuid()::text))
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DROP TABLE system_settings;
