-- +goose Up
-- +goose StatementBegin
CREATE TABLE teldrive.shared_files (
  id TEXT PRIMARY KEY NOT NULL DEFAULT teldrive.generate_uid(16),
  file_id TEXT NOT NULL,
  shared_with_username TEXT,
  created_at TIMESTAMP NOT NULL DEFAULT timezone('utc'::text, now()),
  updated_at TIMESTAMP NOT NULL DEFAULT timezone('utc'::text, now()),
  CONSTRAINT unique_token_file UNIQUE (shared_with_username, file_id),
  CONSTRAINT fk_shared_files_files FOREIGN KEY (file_id) REFERENCES teldrive.files (id)
);

ALTER TABLE teldrive.files
ADD COLUMN visibility VARCHAR(10) NOT NULL CHECK (visibility IN ('public', 'private', 'limited')) DEFAULT 'private';

CREATE FUNCTION teldrive.add_shared_users(file_id_param TEXT, usernames_param TEXT[]) RETURNS VOID AS $$
DECLARE
    i INT;
BEGIN
  FOR i IN 1..array_length(usernames_param, 1) LOOP
    INSERT INTO teldrive.shared_files (file_id, shared_with_username)
    VALUES (file_id_param, usernames_param[i]);
  END LOOP;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION teldrive.remove_shared_users(file_id_param TEXT, usernames_param TEXT[]) RETURNS VOID AS $$
BEGIN
  DELETE FROM teldrive.shared_files
  WHERE teldrive.shared_files.file_id = file_id_param
  AND teldrive.shared_files.shared_with_username = ANY(usernames_param);
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE teldrive.files
DROP COLUMN IF EXISTS visibility;

DROP TABLE IF EXISTS teldrive.shared_files;

DROP FUNCTION IF EXISTS teldrive.add_shared_users;
DROP FUNCTION IF EXISTS teldrive.remove_shared_users;
-- +goose StatementEnd
