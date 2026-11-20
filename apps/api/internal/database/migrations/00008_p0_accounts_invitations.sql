-- +goose Up
-- P0 product delivery: account email/status and invitations for people who
-- have not registered yet. Existing identity and invitation history remain
-- untouched; the new columns are additive.

ALTER TABLE users ADD COLUMN email text;
ALTER TABLE users ADD COLUMN is_active boolean NOT NULL DEFAULT true;

CREATE UNIQUE INDEX users_email_unique ON users (lower(email))
    WHERE email IS NOT NULL AND btrim(email) <> '';

-- Invitations may now target a username before the account exists. The
-- inviter still has to be a team owner/admin; registration claims the pending
-- row by username, so an invited person can join right after signing up.
ALTER TABLE team_invitations ALTER COLUMN invited_user_id DROP NOT NULL;
ALTER TABLE team_invitations ADD COLUMN invited_username text NOT NULL DEFAULT '';

UPDATE team_invitations i
SET invited_username = u.username
FROM users u
WHERE u.id = i.invited_user_id AND i.invited_username = '';

ALTER TABLE team_invitations
    ADD CONSTRAINT team_invitations_target_present
    CHECK (invited_user_id IS NOT NULL OR btrim(invited_username) <> '');

CREATE UNIQUE INDEX team_invitations_one_pending_username
    ON team_invitations (team_id, lower(invited_username))
    WHERE status = 'pending' AND invited_user_id IS NULL;

COMMENT ON COLUMN users.email IS 'Optional unique contact email; registration and profile settings.';
COMMENT ON COLUMN users.is_active IS 'Disabled accounts cannot sign in and their sessions stop authenticating.';
COMMENT ON COLUMN team_invitations.invited_username IS 'Username used to claim the invitation after registration.';

-- +goose Down
DROP INDEX IF EXISTS team_invitations_one_pending_username;
ALTER TABLE team_invitations DROP CONSTRAINT IF EXISTS team_invitations_target_present;
-- Pending invitations for people who never registered cannot be restored to
-- the old NOT NULL shape; only those unclaimed rows are removed on rollback.
DELETE FROM team_invitations WHERE invited_user_id IS NULL AND status = 'pending';
ALTER TABLE team_invitations DROP COLUMN IF EXISTS invited_username;
ALTER TABLE team_invitations ALTER COLUMN invited_user_id SET NOT NULL;
DROP INDEX IF EXISTS users_email_unique;
ALTER TABLE users DROP COLUMN IF EXISTS is_active;
ALTER TABLE users DROP COLUMN IF EXISTS email;
