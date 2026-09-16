//go:build integration

package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"github.com/tgdrive/teldrive/v2/internal/testutil/querytrace"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

func TestFindResumableUploadUsesTargetedQueries(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, "INSERT INTO channels (channel_id,user_id,name,selected) VALUES (9001,1001,'storage',true)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO upload_sessions (user_id,name,normalized_name,expected_size,mime_type,mod_time,encryption,conflict_policy,part_size,expires_at)
SELECT 1001, 'other-' || value, 'other-' || value, 10, 'application/octet-stream', now(), false, 'replace', 10, now() + interval '1 day'
FROM generate_series(1,500) AS value`); err != nil {
		t.Fatal(err)
	}
	matchingID := uuid.New()
	modTime := time.Now().UTC().Truncate(time.Second)
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO upload_sessions (id,user_id,name,normalized_name,expected_size,mime_type,mod_time,encryption,conflict_policy,part_size,expires_at)
VALUES ($1,1001,'target.bin','target.bin',10,'application/octet-stream',$2,false,'replace',10,now() + interval '1 day')`, matchingID, modTime); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO upload_parts (upload_id,part_no,channel_id,message_id,plain_size,stored_size,state)
VALUES ($1,1,9001,77,10,10,'stored')`, matchingID); err != nil {
		t.Fatal(err)
	}

	tracer := &querytrace.Counter{}
	config, err := pgxpool.ParseConfig(db.URL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	worker := &UploadSourceWorker{queries: sqlcgen.New(pool), uploads: uploads.NewService(pool)}
	session, stored, err := worker.findResumableUpload(ctx, 1001, nil, "target.bin", UploadFileSource{
		Size: 10, MIMEType: "application/octet-stream", ModTime: modTime, HasModTime: true,
	}, false)
	if err != nil {
		t.Fatalf("findResumableUpload() error = %v", err)
	}
	if session == nil || session.ID.Bytes != matchingID || stored[1] != 10 {
		t.Fatalf("resumable upload = %#v, stored = %#v", session, stored)
	}
	for _, name := range []string{"FindResumableUploadSessions", "ListUploadPartsByUploadIDs"} {
		if got := tracer.Count(name); got != 1 {
			t.Fatalf("%s queries = %d, want 1", name, got)
		}
	}
	if got := tracer.Count("ListUploadSessions") + tracer.Count("ListUploadParts"); got != 0 {
		t.Fatalf("paged resumable queries = %d, want 0", got)
	}
}
