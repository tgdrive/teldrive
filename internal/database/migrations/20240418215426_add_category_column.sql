-- +goose Up
-- +goose StatementBegin
ALTER TABLE "teldrive"."files" ADD COLUMN "category" text;
CREATE INDEX IF NOT EXISTS "files_category_type_user_id_index" ON "teldrive"."files" ("category","type","user_id");
UPDATE teldrive.files
SET category =
    CASE
        WHEN name ILIKE '%.doc%' OR name ILIKE '%.docx%' OR name ILIKE '%.ppt%' OR name ILIKE '%.pptx%' OR
             name ILIKE '%.pps%' OR name ILIKE '%.ppsx%' OR name ILIKE '%.odt%' OR name ILIKE '%.xls%' OR
             name ILIKE '%.xlsx%' OR name ILIKE '%.csv%' OR name ILIKE '%.pdf%' OR name ILIKE '%.txt%'
            THEN 'document'
        WHEN name ILIKE '%.jpg%' OR name ILIKE '%.jpeg%' OR name ILIKE '%.png%' OR
             name ILIKE '%.gif%' OR name ILIKE '%.bmp%' OR name ILIKE '%.svg%'
            THEN 'image'
        WHEN name ILIKE '%.mp4%' OR name ILIKE '%.webm%' OR name ILIKE '%.mov%' OR
             name ILIKE '%.avi%' OR name ILIKE '%.m4v%' OR name ILIKE '%.flv%' OR
             name ILIKE '%.wmv%' OR name ILIKE '%.mkv%' OR name ILIKE '%.mpg%' OR
             name ILIKE '%.mpeg%' OR name ILIKE '%.m2v%' OR name ILIKE '%.mpv%'
            THEN 'video'
        WHEN name ILIKE '%.mp3%' OR name ILIKE '%.wav%' OR name ILIKE '%.ogg%' OR
             name ILIKE '%.m4a%' OR name ILIKE '%.flac%' OR name ILIKE '%.aac%' OR
             name ILIKE '%.wma%' OR name ILIKE '%.aiff%' OR name ILIKE '%.ape%' OR
             name ILIKE '%.alac%' OR name ILIKE '%.opus%' OR name ILIKE '%.pcm%'
            THEN 'audio'
        WHEN name ILIKE '%.zip%' OR name ILIKE '%.rar%' OR name ILIKE '%.tar%' OR name ILIKE '%.gz%' OR
             name ILIKE '%.7z%' OR name ILIKE '%.iso%' OR name ILIKE '%.dmg%' OR name ILIKE '%.pkg%'
            THEN 'archive'
        ELSE 'other'
    END
WHERE type = 'file';

-- +goose StatementEnd