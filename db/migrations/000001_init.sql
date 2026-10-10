-- +goose Up

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;

CREATE TYPE /* TEMPLATE: schema */file_kind AS ENUM ('file', 'folder');
CREATE TYPE /* TEMPLATE: schema */file_status AS ENUM ('active', 'trashed', 'deletion_pending');
CREATE TYPE /* TEMPLATE: schema */name_conflict_policy AS ENUM ('fail', 'replace', 'rename');
CREATE TYPE /* TEMPLATE: schema */upload_state AS ENUM ('open', 'completing', 'completed', 'aborted', 'expired');
CREATE TYPE /* TEMPLATE: schema */upload_part_state AS ENUM ('reserved', 'uploading', 'stored', 'failed');
CREATE TYPE /* TEMPLATE: schema */channel_health AS ENUM ('unknown', 'healthy', 'degraded', 'unavailable');
CREATE TYPE /* TEMPLATE: schema */telegram_login_method AS ENUM ('phone', 'qr');

CREATE TABLE /* TEMPLATE: schema */users (
    user_id BIGINT PRIMARY KEY,
    display_name TEXT,
    username TEXT,
    premium BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE /* TEMPLATE: schema */telegram_login_flows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    method /* TEMPLATE: schema */telegram_login_method NOT NULL,
    phone_number_ciphertext BYTEA,
    telegram_state_ciphertext BYTEA NOT NULL,
    password_required BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT telegram_login_flows_phone_required CHECK (
        (method = 'phone' AND phone_number_ciphertext IS NOT NULL)
        OR (method = 'qr' AND phone_number_ciphertext IS NULL)
    )
);

CREATE INDEX telegram_login_flows_expiry_idx
    ON /* TEMPLATE: schema */telegram_login_flows (expires_at)
    WHERE completed_at IS NULL;

CREATE TABLE /* TEMPLATE: schema */sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id BIGINT NOT NULL REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    telegram_session BYTEA NOT NULL,
    refresh_token_hash BYTEA NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX sessions_user_active_idx
    ON /* TEMPLATE: schema */sessions (user_id, created_at DESC)
    WHERE revoked_at IS NULL;

CREATE TABLE /* TEMPLATE: schema */api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id BIGINT NOT NULL REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    secret_hash BYTEA NOT NULL UNIQUE,
    last_used_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT api_keys_name_not_blank CHECK (length(btrim(name)) > 0)
);

CREATE INDEX api_keys_user_active_idx
    ON /* TEMPLATE: schema */api_keys (user_id, created_at DESC)
    WHERE revoked_at IS NULL;

CREATE TABLE /* TEMPLATE: schema */bots (
    bot_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    username TEXT,
    token_ciphertext BYTEA NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    session BYTEA,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    last_used_at TIMESTAMPTZ,
    retry_after TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, bot_id),
    CONSTRAINT bots_consecutive_failures_nonnegative CHECK (consecutive_failures >= 0)
);


CREATE INDEX bots_upload_eligible_idx
    ON /* TEMPLATE: schema */bots (user_id, bot_id)
    WHERE enabled;

CREATE TABLE /* TEMPLATE: schema */bot_selection_counters (
    user_id BIGINT NOT NULL REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    operation TEXT NOT NULL,
    next_value BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, operation),
    CONSTRAINT bot_selection_operation_valid CHECK (operation IN ('upload', 'download')),
    CONSTRAINT bot_selection_next_value_nonnegative CHECK (next_value >= 0)
);
CREATE TABLE /* TEMPLATE: schema */channels (
    channel_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    selected BOOLEAN NOT NULL DEFAULT FALSE,
    health /* TEMPLATE: schema */channel_health NOT NULL DEFAULT 'unknown',
    last_checked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, channel_id),
    CONSTRAINT channels_name_not_blank CHECK (length(btrim(name)) > 0)
);

CREATE UNIQUE INDEX channels_one_selected_per_user_idx
    ON /* TEMPLATE: schema */channels (user_id)
    WHERE selected;

CREATE TABLE /* TEMPLATE: schema */files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id BIGINT NOT NULL REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    parent_id UUID,
    name TEXT NOT NULL,
    normalized_name TEXT NOT NULL,
    kind /* TEMPLATE: schema */file_kind NOT NULL,
    mime_type TEXT,
    size BIGINT,
    hash_algorithm TEXT,
    hash_value TEXT,
    encryption BOOLEAN NOT NULL DEFAULT FALSE,
    encryption_key_version INTEGER,
    status /* TEMPLATE: schema */file_status NOT NULL DEFAULT 'active',
    mod_time TIMESTAMPTZ NOT NULL,
    generation BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    UNIQUE (id, user_id),
    FOREIGN KEY (parent_id, user_id)
        REFERENCES /* TEMPLATE: schema */files(id, user_id)
        ON DELETE RESTRICT,
    CONSTRAINT files_name_not_blank CHECK (length(btrim(name)) > 0),
    CONSTRAINT files_normalized_name_not_blank CHECK (length(normalized_name) > 0),
    CONSTRAINT files_kind_size_check CHECK (
        (kind = 'folder' AND size IS NULL)
        OR
        (kind = 'file' AND size IS NOT NULL AND size >= 0)
    ),
    CONSTRAINT files_encryption_key_check CHECK (
        (NOT encryption AND encryption_key_version IS NULL)
        OR
        (encryption AND encryption_key_version IS NOT NULL)
    ),
    CONSTRAINT files_hash_pair_check CHECK (
        (hash_algorithm IS NULL AND hash_value IS NULL)
        OR
        (hash_algorithm IS NOT NULL AND hash_value IS NOT NULL)
    ),
    CONSTRAINT files_deleted_at_check CHECK (
        (status = 'active' AND deleted_at IS NULL)
        OR
        (status <> 'active' AND deleted_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX files_unique_active_root_name_idx
    ON /* TEMPLATE: schema */files (user_id, normalized_name)
    WHERE parent_id IS NULL AND status = 'active';

CREATE UNIQUE INDEX files_unique_active_child_name_idx
    ON /* TEMPLATE: schema */files (user_id, parent_id, normalized_name)
    WHERE parent_id IS NOT NULL AND status = 'active';

CREATE INDEX files_list_idx
    ON /* TEMPLATE: schema */files (user_id, parent_id, status, kind, normalized_name, id);

CREATE INDEX files_name_search_idx
    ON /* TEMPLATE: schema */files USING gin (normalized_name gin_trgm_ops);

CREATE TABLE /* TEMPLATE: schema */file_parts (
    file_id UUID NOT NULL REFERENCES /* TEMPLATE: schema */files(id) ON DELETE CASCADE,
    part_no INTEGER NOT NULL,
    channel_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    plain_size BIGINT,
    stored_size BIGINT,
    checksum TEXT,
    salt TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (file_id, part_no),
    UNIQUE (channel_id, message_id),
    CONSTRAINT file_parts_part_no_positive CHECK (part_no > 0),
    CONSTRAINT file_parts_plain_size_nonnegative CHECK (plain_size >= 0),
    CONSTRAINT file_parts_stored_size_nonnegative CHECK (stored_size >= 0)
);

CREATE INDEX file_parts_channel_idx
    ON /* TEMPLATE: schema */file_parts (channel_id, message_id);

CREATE TABLE /* TEMPLATE: schema */upload_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id BIGINT NOT NULL REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    parent_id UUID,
    name TEXT NOT NULL,
    normalized_name TEXT NOT NULL,
    expected_size BIGINT NOT NULL,
    expected_hash_algorithm TEXT,
    expected_hash_value TEXT,
    mime_type TEXT,
    mod_time TIMESTAMPTZ NOT NULL,
    encryption BOOLEAN NOT NULL DEFAULT FALSE,
    encryption_key_version INTEGER,
    conflict_policy /* TEMPLATE: schema */name_conflict_policy NOT NULL DEFAULT 'fail',
    part_size BIGINT NOT NULL,
    state /* TEMPLATE: schema */upload_state NOT NULL DEFAULT 'open',
    file_id UUID REFERENCES /* TEMPLATE: schema */files(id) ON DELETE SET NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    FOREIGN KEY (parent_id, user_id)
        REFERENCES /* TEMPLATE: schema */files(id, user_id)
        ON DELETE RESTRICT,
    CONSTRAINT upload_sessions_name_not_blank CHECK (length(btrim(name)) > 0),
    CONSTRAINT upload_sessions_normalized_name_not_blank CHECK (length(normalized_name) > 0),
    CONSTRAINT upload_sessions_expected_size_nonnegative CHECK (expected_size >= 0),
    CONSTRAINT upload_sessions_part_size_positive CHECK (part_size > 0),
    CONSTRAINT upload_sessions_hash_pair_check CHECK (
        (expected_hash_algorithm IS NULL AND expected_hash_value IS NULL)
        OR
        (expected_hash_algorithm IS NOT NULL AND expected_hash_value IS NOT NULL)
    ),
    CONSTRAINT upload_sessions_encryption_key_check CHECK (
        (NOT encryption AND encryption_key_version IS NULL)
        OR
        (encryption AND encryption_key_version IS NOT NULL)
    ),
    CONSTRAINT upload_sessions_completion_check CHECK (
        (state = 'completed' AND completed_at IS NOT NULL)
        OR
        (state <> 'completed' AND completed_at IS NULL)
    )
);

CREATE INDEX upload_sessions_user_state_idx
    ON /* TEMPLATE: schema */upload_sessions (user_id, state, created_at DESC, id);

CREATE INDEX upload_sessions_expiry_idx
    ON /* TEMPLATE: schema */upload_sessions (expires_at)
    WHERE state IN ('open', 'completing');

CREATE TABLE /* TEMPLATE: schema */upload_parts (
    upload_id UUID NOT NULL REFERENCES /* TEMPLATE: schema */upload_sessions(id) ON DELETE CASCADE,
    part_no INTEGER NOT NULL,
    channel_id BIGINT NOT NULL,
    message_id BIGINT,
    plain_size BIGINT NOT NULL,
    stored_size BIGINT,
    checksum TEXT,
    salt TEXT,
    state /* TEMPLATE: schema */upload_part_state NOT NULL DEFAULT 'reserved',
    lease_token UUID,
    lease_expires_at TIMESTAMPTZ,
    last_error_code TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (upload_id, part_no),
    CONSTRAINT upload_parts_part_no_positive CHECK (part_no > 0),
    CONSTRAINT upload_parts_plain_size_nonnegative CHECK (plain_size >= 0),
    CONSTRAINT upload_parts_stored_size_nonnegative CHECK (stored_size IS NULL OR stored_size >= 0),
    CONSTRAINT upload_parts_storage_state_check CHECK (
        (state = 'stored' AND message_id IS NOT NULL AND stored_size IS NOT NULL)
        OR
        (state <> 'stored')
    ),
    CONSTRAINT upload_parts_lease_pair_check CHECK (
        (lease_token IS NULL AND lease_expires_at IS NULL)
        OR
        (lease_token IS NOT NULL AND lease_expires_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX upload_parts_unique_message_idx
    ON /* TEMPLATE: schema */upload_parts (channel_id, message_id)
    WHERE message_id IS NOT NULL;

CREATE INDEX upload_parts_expired_lease_idx
    ON /* TEMPLATE: schema */upload_parts (lease_expires_at)
    WHERE state IN ('reserved', 'uploading') AND lease_expires_at IS NOT NULL;

CREATE TABLE /* TEMPLATE: schema */file_shares (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    file_id UUID NOT NULL REFERENCES /* TEMPLATE: schema */files(id) ON DELETE CASCADE,
    owner_id BIGINT NOT NULL REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    token_prefix TEXT NOT NULL,
    token_hash BYTEA NOT NULL UNIQUE,
    password_hash TEXT,
    expires_at TIMESTAMPTZ,
    max_downloads BIGINT,
    download_count BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    CONSTRAINT file_shares_max_downloads_positive CHECK (max_downloads IS NULL OR max_downloads > 0),
    CONSTRAINT file_shares_download_count_nonnegative CHECK (download_count >= 0)
);

CREATE INDEX file_shares_owner_active_idx
    ON /* TEMPLATE: schema */file_shares (owner_id, file_id, created_at DESC)
    WHERE revoked_at IS NULL;

CREATE INDEX file_shares_expiry_idx
    ON /* TEMPLATE: schema */file_shares (expires_at)
    WHERE revoked_at IS NULL AND expires_at IS NOT NULL;

CREATE TABLE /* TEMPLATE: schema */idempotency_keys (
    user_id BIGINT NOT NULL REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    scope TEXT NOT NULL,
    key UUID NOT NULL,
    request_hash BYTEA NOT NULL,
    resource_type TEXT,
    resource_id TEXT,
    response_ciphertext BYTEA,
    completed_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, scope, key),
    CONSTRAINT idempotency_scope_not_blank CHECK (length(btrim(scope)) > 0)
);

CREATE INDEX idempotency_keys_expiry_idx
    ON /* TEMPLATE: schema */idempotency_keys (expires_at);

CREATE TABLE /* TEMPLATE: schema */audit_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id BIGINT REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT,
    metadata JSONB,
    request_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT audit_events_action_not_blank CHECK (length(btrim(action)) > 0),
    CONSTRAINT audit_events_resource_type_not_blank CHECK (length(btrim(resource_type)) > 0)
);

CREATE INDEX audit_events_user_created_idx
    ON /* TEMPLATE: schema */audit_events (user_id, created_at DESC, id);

-- RiverPro owns and migrates its own tables. TelDrive does not duplicate them here.

-- +goose Down

DROP TABLE IF EXISTS /* TEMPLATE: schema */audit_events;
DROP TABLE IF EXISTS /* TEMPLATE: schema */idempotency_keys;
DROP TABLE IF EXISTS /* TEMPLATE: schema */file_shares;
DROP TABLE IF EXISTS /* TEMPLATE: schema */upload_parts;
DROP TABLE IF EXISTS /* TEMPLATE: schema */upload_sessions;
DROP TABLE IF EXISTS /* TEMPLATE: schema */file_parts;
DROP TABLE IF EXISTS /* TEMPLATE: schema */files;
DROP TABLE IF EXISTS /* TEMPLATE: schema */channels;
DROP TABLE IF EXISTS /* TEMPLATE: schema */bot_selection_counters;
DROP TABLE IF EXISTS /* TEMPLATE: schema */bots;
DROP TABLE IF EXISTS /* TEMPLATE: schema */api_keys;
DROP TABLE IF EXISTS /* TEMPLATE: schema */sessions;
DROP TABLE IF EXISTS /* TEMPLATE: schema */telegram_login_flows;
DROP TABLE IF EXISTS /* TEMPLATE: schema */users;

DROP TYPE IF EXISTS /* TEMPLATE: schema */telegram_login_method;
DROP TYPE IF EXISTS /* TEMPLATE: schema */channel_health;
DROP TYPE IF EXISTS /* TEMPLATE: schema */upload_part_state;
DROP TYPE IF EXISTS /* TEMPLATE: schema */upload_state;
DROP TYPE IF EXISTS /* TEMPLATE: schema */name_conflict_policy;
DROP TYPE IF EXISTS /* TEMPLATE: schema */file_status;
DROP TYPE IF EXISTS /* TEMPLATE: schema */file_kind;
