-- +goose Up
ALTER TABLE teldrive.channels ADD COLUMN IF NOT EXISTS created_at timestamptz;

UPDATE teldrive.channels c
SET created_at = COALESCE(
    (SELECT MIN(f.created_at) FROM teldrive.files f WHERE f.channel_id = c.channel_id),
    NOW() - INTERVAL '1 day'
)
WHERE c.created_at IS NULL;

ALTER TABLE teldrive.channels ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE teldrive.channels ALTER COLUMN created_at SET DEFAULT timezone('utc'::text,now());

-- +goose Down
ALTER TABLE teldrive.channels DROP COLUMN IF EXISTS created_at;
