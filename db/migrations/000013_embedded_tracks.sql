-- +goose Up

-- Player options for audio tracks and subtitles stored inside .mkv and .webm files.
--   embedded_tracks            read them in the web player (off by default: nothing extra is loaded)
--   embedded_tracks_on_shares  also on this account's public share links; visitors have no settings
--                              of their own, so the owner decides. Only meaningful while
--                              embedded_tracks is on, and the update query keeps it off otherwise.
-- Written so that running it again is harmless (a database that already has these columns, for
-- example one that got them from a build made before this migration was released, comes through
-- unchanged, values included): the columns are added only if missing, and the constraint is replaced
-- in the same statement.
ALTER TABLE /* TEMPLATE: schema */user_preferences
    ADD COLUMN IF NOT EXISTS embedded_tracks BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS embedded_tracks_on_shares BOOLEAN NOT NULL DEFAULT false,
    DROP CONSTRAINT IF EXISTS user_preferences_embedded_tracks_on_shares_needs_tracks,
    ADD CONSTRAINT user_preferences_embedded_tracks_on_shares_needs_tracks
        CHECK (NOT embedded_tracks_on_shares OR embedded_tracks);

-- +goose Down

ALTER TABLE /* TEMPLATE: schema */user_preferences
    DROP CONSTRAINT IF EXISTS user_preferences_embedded_tracks_on_shares_needs_tracks,
    DROP COLUMN IF EXISTS embedded_tracks_on_shares,
    DROP COLUMN IF EXISTS embedded_tracks;
