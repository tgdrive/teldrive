//go:build integration

package fileops

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/channels"
	"github.com/tgdrive/teldrive/v2/internal/localtelegram"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestPurgeThroughLocalTelegramStorage(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	server, err := localtelegram.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := localtelegram.NewRunner(server)
	if err != nil {
		t.Fatal(err)
	}
	storage := telegramstore.NewGotdStorage(runner)
	channel, err := storage.CreateChannel(ctx, 1001, "storage")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, "INSERT INTO channels (channel_id,user_id,name,selected) VALUES ($1,1001,'storage',true)", channel.ID); err != nil {
		t.Fatal(err)
	}
	payload := []byte("local purge")
	stored, err := storage.Upload(ctx, telegramstore.UploadRequest{
		UserID: 1001, ChannelID: channel.ID, Name: "purge.bin",
		Reader: bytes.NewReader(payload), Size: int64(len(payload)), Threads: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	fileID := uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,name,normalized_name,kind,mime_type,size,encryption,status,mod_time)
VALUES ($1,1001,'purge.bin','purge.bin','file','application/octet-stream',$2,false,'active',now())
`, fileID, len(payload)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO file_parts (file_id,part_no,channel_id,message_id,plain_size,stored_size)
VALUES ($1,1,$2,$3,$4,$5)
`, fileID, stored.ChannelID, stored.MessageID, stored.Size, stored.Size); err != nil {
		t.Fatal(err)
	}
	uploadID := uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO upload_sessions (
    id,user_id,name,normalized_name,expected_size,mime_type,mod_time,
    encryption,conflict_policy,part_size,state,file_id,expires_at,completed_at
) VALUES (
    $1,1001,'purge.bin','purge.bin',$2,'application/octet-stream',now(),
    false,'replace',1048576,'completed',$3,now()+interval '1 day',now()
)
`, uploadID, len(payload), fileID); err != nil {
		t.Fatal(err)
	}
	catalogService := catalog.NewService(db.Pool)
	channelService := channels.NewService(db.Pool, nil, channels.Config{PartLimit: 100})
	service, err := NewService(db.Pool, catalogService, channelService, storage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalogService.Trash(ctx, 1001, fileID); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			errs <- service.Purge(ctx, 1001, fileID)
		}()
	}
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent Purge() error = %v", err)
		}
	}
	var state string
	var retainedFileID *uuid.UUID
	if err := db.Pool.QueryRow(ctx, "SELECT state::text, file_id FROM upload_sessions WHERE id = $1", uploadID).Scan(&state, &retainedFileID); err != nil {
		t.Fatalf("read retained upload session: %v", err)
	}
	if state != "completed" || retainedFileID != nil {
		t.Fatalf("retained upload session = state %q, file_id %v", state, retainedFileID)
	}
}
