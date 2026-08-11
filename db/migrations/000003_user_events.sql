-- +goose Up

CREATE TABLE /* TEMPLATE: schema */user_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT,
    generation BIGINT,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT user_events_event_type_not_blank CHECK (length(btrim(event_type)) > 0),
    CONSTRAINT user_events_resource_type_not_blank CHECK (length(btrim(resource_type)) > 0),
    CONSTRAINT user_events_payload_object CHECK (jsonb_typeof(payload) = 'object')
);

CREATE INDEX user_events_user_cursor_idx
    ON /* TEMPLATE: schema */user_events (user_id, id);

CREATE INDEX user_events_retention_idx
    ON /* TEMPLATE: schema */user_events (occurred_at, id);

CREATE TABLE /* TEMPLATE: schema */user_event_stream_state (
    user_id BIGINT PRIMARY KEY REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    last_event_id BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE /* TEMPLATE: schema */event_stream_tickets (
    token_hash BYTEA PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES /* TEMPLATE: schema */users(user_id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX event_stream_tickets_expiry_idx
    ON /* TEMPLATE: schema */event_stream_tickets (expires_at);

-- +goose StatementBegin
CREATE FUNCTION /* TEMPLATE: schema */notify_user_event() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO /* TEMPLATE: schema */user_event_stream_state (user_id, last_event_id, updated_at)
    VALUES (NEW.user_id, NEW.id, now())
    ON CONFLICT (user_id) DO UPDATE
    SET last_event_id = EXCLUDED.last_event_id,
        updated_at = EXCLUDED.updated_at;

    PERFORM pg_notify(
        'teldrive_events',
        json_build_object('user_id', NEW.user_id)::text
    );
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER user_events_notify_after_insert
AFTER INSERT ON /* TEMPLATE: schema */user_events
FOR EACH ROW EXECUTE FUNCTION /* TEMPLATE: schema */notify_user_event();

-- +goose StatementBegin
CREATE FUNCTION /* TEMPLATE: schema */emit_file_event() RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    row_value /* TEMPLATE: schema */files%ROWTYPE;
    event_name TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        row_value := OLD;
        event_name := 'file.purged';
    ELSIF TG_OP = 'INSERT' THEN
        row_value := NEW;
        event_name := 'file.created';
    ELSE
        row_value := NEW;
        IF ROW(
            OLD.parent_id, OLD.name, OLD.kind, OLD.mime_type, OLD.size,
            OLD.hash_algorithm, OLD.hash_value, OLD.encryption,
            OLD.encryption_key_version, OLD.status, OLD.mod_time, OLD.generation,
            OLD.deleted_at
        ) IS NOT DISTINCT FROM ROW(
            NEW.parent_id, NEW.name, NEW.kind, NEW.mime_type, NEW.size,
            NEW.hash_algorithm, NEW.hash_value, NEW.encryption,
            NEW.encryption_key_version, NEW.status, NEW.mod_time, NEW.generation,
            NEW.deleted_at
        ) THEN
            RETURN NEW;
        END IF;

        event_name := CASE
            WHEN OLD.status IS DISTINCT FROM NEW.status AND NEW.status = 'trashed' THEN 'file.trashed'
            WHEN OLD.status IS DISTINCT FROM NEW.status AND NEW.status = 'deletion_pending' THEN 'file.deletion_pending'
            WHEN OLD.status IS DISTINCT FROM NEW.status AND NEW.status = 'active' THEN 'file.restored'
            ELSE 'file.updated'
        END;
    END IF;

    INSERT INTO /* TEMPLATE: schema */user_events (
        user_id, event_type, resource_type, resource_id, generation, payload
    ) VALUES (
        row_value.user_id,
        event_name,
        'file',
        row_value.id::text,
        row_value.generation,
        jsonb_strip_nulls(jsonb_build_object(
            'parentId', row_value.parent_id,
            'name', row_value.name,
            'kind', row_value.kind,
            'status', row_value.status
        ))
    );

    RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER files_emit_user_event
AFTER INSERT OR UPDATE OR DELETE ON /* TEMPLATE: schema */files
FOR EACH ROW EXECUTE FUNCTION /* TEMPLATE: schema */emit_file_event();

-- +goose StatementBegin
CREATE FUNCTION /* TEMPLATE: schema */emit_upload_event() RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    event_name TEXT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        event_name := 'upload.created';
    ELSIF OLD.state IS NOT DISTINCT FROM NEW.state THEN
        RETURN NEW;
    ELSE
        event_name := 'upload.' || NEW.state::text;
    END IF;

    INSERT INTO /* TEMPLATE: schema */user_events (
        user_id, event_type, resource_type, resource_id, payload
    ) VALUES (
        NEW.user_id,
        event_name,
        'upload',
        NEW.id::text,
        jsonb_strip_nulls(jsonb_build_object(
            'state', NEW.state,
            'fileId', NEW.file_id,
            'name', NEW.name
        ))
    );

    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER upload_sessions_emit_user_event
AFTER INSERT OR UPDATE OF state ON /* TEMPLATE: schema */upload_sessions
FOR EACH ROW EXECUTE FUNCTION /* TEMPLATE: schema */emit_upload_event();

-- +goose StatementBegin
CREATE FUNCTION /* TEMPLATE: schema */emit_share_event() RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    row_value /* TEMPLATE: schema */file_shares%ROWTYPE;
    event_name TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        row_value := OLD;
        event_name := 'share.deleted';
    ELSIF TG_OP = 'INSERT' THEN
        row_value := NEW;
        event_name := 'share.created';
    ELSE
        row_value := NEW;
        IF ROW(OLD.password_hash, OLD.expires_at, OLD.max_downloads, OLD.revoked_at)
            IS NOT DISTINCT FROM
           ROW(NEW.password_hash, NEW.expires_at, NEW.max_downloads, NEW.revoked_at) THEN
            RETURN NEW;
        END IF;
        event_name := CASE
            WHEN OLD.revoked_at IS NULL AND NEW.revoked_at IS NOT NULL THEN 'share.revoked'
            ELSE 'share.updated'
        END;
    END IF;

    INSERT INTO /* TEMPLATE: schema */user_events (
        user_id, event_type, resource_type, resource_id, payload
    ) VALUES (
        row_value.owner_id,
        event_name,
        'share',
        row_value.id::text,
        jsonb_strip_nulls(jsonb_build_object(
            'fileId', row_value.file_id,
            'expiresAt', row_value.expires_at,
            'revokedAt', row_value.revoked_at
        ))
    );

    RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER file_shares_emit_user_event
AFTER INSERT OR UPDATE OR DELETE ON /* TEMPLATE: schema */file_shares
FOR EACH ROW EXECUTE FUNCTION /* TEMPLATE: schema */emit_share_event();

-- +goose StatementBegin
CREATE FUNCTION /* TEMPLATE: schema */emit_channel_event() RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    row_value /* TEMPLATE: schema */channels%ROWTYPE;
    event_name TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        row_value := OLD;
        event_name := 'channel.deleted';
    ELSIF TG_OP = 'INSERT' THEN
        row_value := NEW;
        event_name := 'channel.created';
    ELSE
        row_value := NEW;
        IF ROW(OLD.name, OLD.selected, OLD.health)
            IS NOT DISTINCT FROM
           ROW(NEW.name, NEW.selected, NEW.health) THEN
            RETURN NEW;
        END IF;
        event_name := 'channel.updated';
    END IF;

    INSERT INTO /* TEMPLATE: schema */user_events (
        user_id, event_type, resource_type, resource_id, payload
    ) VALUES (
        row_value.user_id,
        event_name,
        'channel',
        row_value.channel_id::text,
        jsonb_build_object(
            'name', row_value.name,
            'selected', row_value.selected,
            'health', row_value.health
        )
    );

    RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER channels_emit_user_event
AFTER INSERT OR UPDATE OR DELETE ON /* TEMPLATE: schema */channels
FOR EACH ROW EXECUTE FUNCTION /* TEMPLATE: schema */emit_channel_event();

-- +goose Down

DROP TRIGGER IF EXISTS channels_emit_user_event ON /* TEMPLATE: schema */channels;
DROP FUNCTION IF EXISTS /* TEMPLATE: schema */emit_channel_event();
DROP TRIGGER IF EXISTS file_shares_emit_user_event ON /* TEMPLATE: schema */file_shares;
DROP FUNCTION IF EXISTS /* TEMPLATE: schema */emit_share_event();
DROP TRIGGER IF EXISTS upload_sessions_emit_user_event ON /* TEMPLATE: schema */upload_sessions;
DROP FUNCTION IF EXISTS /* TEMPLATE: schema */emit_upload_event();
DROP TRIGGER IF EXISTS files_emit_user_event ON /* TEMPLATE: schema */files;
DROP FUNCTION IF EXISTS /* TEMPLATE: schema */emit_file_event();
DROP TRIGGER IF EXISTS user_events_notify_after_insert ON /* TEMPLATE: schema */user_events;
DROP FUNCTION IF EXISTS /* TEMPLATE: schema */notify_user_event();
DROP TABLE IF EXISTS /* TEMPLATE: schema */event_stream_tickets;
DROP TABLE IF EXISTS /* TEMPLATE: schema */user_events;
DROP TABLE IF EXISTS /* TEMPLATE: schema */user_event_stream_state;
