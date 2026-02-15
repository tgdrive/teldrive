-- Add created_at column to channels table
ALTER TABLE teldrive.channels ADD COLUMN IF NOT EXISTS created_at timestamptz;

-- Set created_at for existing channels that don't have it
-- Use the earliest file created_at as reference, or a reasonable default if no files
UPDATE teldrive.channels c
SET created_at = COALESCE(
    (SELECT MIN(f.created_at) FROM teldrive.files f WHERE f.channel_id = c.channel_id),
    NOW() - INTERVAL '1 day'
)
WHERE c.created_at IS NULL;

-- Make column not-nullable and set default for new records
ALTER TABLE teldrive.channels ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE teldrive.channels ALTER COLUMN created_at SET DEFAULT timezone('utc'::text,now());
