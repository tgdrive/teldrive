-- +goose Up

CREATE TYPE /* TEMPLATE: schema */user_role AS ENUM ('owner', 'admin', 'user');
CREATE TYPE /* TEMPLATE: schema */share_permission AS ENUM ('read', 'edit');

ALTER TABLE /* TEMPLATE: schema */users
    ADD COLUMN role /* TEMPLATE: schema */user_role NOT NULL DEFAULT 'user',
    ADD COLUMN disabled_at TIMESTAMPTZ;

WITH first_user AS (
    SELECT user_id
    FROM /* TEMPLATE: schema */users
    ORDER BY created_at ASC, user_id ASC
    LIMIT 1
)
UPDATE /* TEMPLATE: schema */users u
SET role = 'owner'
FROM first_user f
WHERE u.user_id = f.user_id;

CREATE INDEX users_role_idx
    ON /* TEMPLATE: schema */users (role, created_at, user_id);


ALTER TABLE /* TEMPLATE: schema */file_shares
    ADD COLUMN permission /* TEMPLATE: schema */share_permission NOT NULL DEFAULT 'read';

CREATE TABLE /* TEMPLATE: schema */file_access_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    file_id UUID NOT NULL REFERENCES /* TEMPLATE: schema */files(id) ON DELETE CASCADE,
    owner_id BIGINT NOT NULL REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    grantee_id BIGINT NOT NULL REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    permission /* TEMPLATE: schema */share_permission NOT NULL DEFAULT 'read',
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    CONSTRAINT file_access_grants_not_self CHECK (owner_id <> grantee_id),
    CONSTRAINT file_access_grants_owner_file FOREIGN KEY (file_id, owner_id)
        REFERENCES /* TEMPLATE: schema */files(id, user_id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX file_access_grants_active_recipient_idx
    ON /* TEMPLATE: schema */file_access_grants (file_id, grantee_id)
    WHERE revoked_at IS NULL;

CREATE INDEX file_access_grants_grantee_idx
    ON /* TEMPLATE: schema */file_access_grants (grantee_id, created_at DESC, id)
    WHERE revoked_at IS NULL;

CREATE INDEX file_access_grants_owner_idx
    ON /* TEMPLATE: schema */file_access_grants (owner_id, file_id, created_at DESC, id)
    WHERE revoked_at IS NULL;

-- +goose Down

DROP INDEX IF EXISTS /* TEMPLATE: schema */file_access_grants_owner_idx;
DROP INDEX IF EXISTS /* TEMPLATE: schema */file_access_grants_grantee_idx;
DROP INDEX IF EXISTS /* TEMPLATE: schema */file_access_grants_active_recipient_idx;
DROP TABLE IF EXISTS /* TEMPLATE: schema */file_access_grants;

ALTER TABLE /* TEMPLATE: schema */file_shares
    DROP COLUMN permission;


DROP INDEX IF EXISTS /* TEMPLATE: schema */users_role_idx;

ALTER TABLE /* TEMPLATE: schema */users
    DROP COLUMN disabled_at,
    DROP COLUMN role;

DROP TYPE IF EXISTS /* TEMPLATE: schema */share_permission;
DROP TYPE IF EXISTS /* TEMPLATE: schema */user_role;
