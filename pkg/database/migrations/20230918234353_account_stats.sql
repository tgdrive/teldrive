-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION teldrive.account_stats(
    IN u_id BIGINT
)  RETURNS TABLE (total_size BIGINT, total_files BIGINT, ch_id BIGINT,ch_name TEXT ) AS $$
DECLARE
    total_size BIGINT;
    total_files BIGINT;
    ch_id BIGINT;
    ch_name TEXT;
BEGIN
    SELECT COUNT(*), SUM(size) into total_files,total_size FROM teldrive.files WHERE user_id=u_id AND type= 'file' and status='active';
    SELECT channel_id ,channel_name into ch_id,ch_name FROM teldrive.channels WHERE selected=TRUE AND user_id=u_id;
    RETURN QUERY SELECT total_size,total_files,ch_id,ch_name;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION IF EXISTS teldrive.account_stats;
-- +goose StatementEnd
