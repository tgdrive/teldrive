//go:build integration

package fileops

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/channels"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestCopyAndPurgeUseIndependentTelegramMessages(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, "INSERT INTO channels (channel_id,user_id,name,selected) VALUES (9001,1001,'storage',true)"); err != nil {
		t.Fatal(err)
	}
	sourceID := insertStoredFile(t, db, "source.bin", 10)
	storage := &fileStorage{messages: map[int64][]byte{10: []byte("data")}, nextID: 100}
	catalogService := catalog.NewService(db.Pool, nil)
	channelService := channels.NewService(db.Pool, nil, channels.Config{PartLimit: 100})
	service, err := NewService(db.Pool, catalogService, channelService, storage)
	if err != nil {
		t.Fatal(err)
	}
	name := "copy.bin"
	copied, err := service.Copy(ctx, CopyInput{UserID: 1001, FileID: sourceID, Name: &name})
	if err != nil {
		t.Fatalf("Copy() error = %v", err)
	}
	copiedID, _ := dbtypes.GoogleUUID(copied.ID)
	parts, err := catalogService.Parts(ctx, 1001, copiedID)
	if err != nil || len(parts) != 1 {
		t.Fatalf("copied parts = %#v, %v", parts, err)
	}
	if parts[0].MessageID == 10 || parts[0].MessageID != 101 {
		t.Fatalf("copied message id = %d", parts[0].MessageID)
	}
	if got := storage.message(101); !bytes.Equal(got, []byte("data")) {
		t.Fatalf("copied Telegram payload = %q", got)
	}

	if _, err := catalogService.Trash(ctx, 1001, sourceID); err != nil {
		t.Fatal(err)
	}
	if err := service.Purge(ctx, 1001, sourceID); err != nil {
		t.Fatalf("Purge() error = %v", err)
	}
	if _, err := catalogService.Get(ctx, 1001, sourceID); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("purged source lookup error = %v", err)
	}
	if got := storage.message(10); got != nil {
		t.Fatalf("source Telegram message survived: %q", got)
	}
	if got := storage.message(101); !bytes.Equal(got, []byte("data")) {
		t.Fatalf("copy was corrupted by purge: %q", got)
	}

	folderID := uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,name,normalized_name,kind,encryption,status,mod_time)
VALUES ($1,1001,'folder','folder','folder',false,'active',now())`, folderID); err != nil {
		t.Fatal(err)
	}
	childID := uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,parent_id,name,normalized_name,kind,mime_type,size,encryption,status,mod_time)
VALUES ($1,1001,$2,'child.bin','child.bin','file','application/octet-stream',4,false,'active',now())`, childID, folderID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO file_parts (file_id,part_no,channel_id,message_id,plain_size,stored_size,checksum,block_hashes)
VALUES ($1,1,9001,30,4,4,repeat('c',64),decode(repeat('cd',32),'hex'))`, childID); err != nil {
		t.Fatal(err)
	}
	storage.mu.Lock()
	storage.messages[30] = []byte("tree")
	storage.mu.Unlock()
	folderName := "folder-copy"
	copiedFolder, err := service.Copy(ctx, CopyInput{UserID: 1001, FileID: folderID, Name: &folderName})
	if err != nil {
		t.Fatalf("Copy(folder) error = %v", err)
	}
	copiedFolderID, _ := dbtypes.GoogleUUID(copiedFolder.ID)
	children, err := catalogService.List(ctx, catalog.ListInput{UserID: 1001, ParentID: &copiedFolderID, Limit: 10})
	if err != nil || len(children) != 1 || children[0].Name != "child.bin" {
		t.Fatalf("copied children = %#v, %v", children, err)
	}
	copiedChildID, _ := dbtypes.GoogleUUID(children[0].ID)
	childParts, err := catalogService.Parts(ctx, 1001, copiedChildID)
	if err != nil || len(childParts) != 1 || childParts[0].MessageID == 30 {
		t.Fatalf("copied child parts = %#v, %v", childParts, err)
	}
	if got := storage.message(childParts[0].MessageID); !bytes.Equal(got, []byte("tree")) {
		t.Fatalf("copied tree payload = %q", got)
	}

	failedID := insertStoredFile(t, db, "failed.bin", 20)
	storage.mu.Lock()
	storage.messages[20] = []byte("fail")
	storage.deleteErr = errors.New("Telegram unavailable")
	storage.mu.Unlock()
	if _, err := catalogService.Trash(ctx, 1001, failedID); err != nil {
		t.Fatal(err)
	}
	if err := service.Purge(ctx, 1001, failedID); err == nil {
		t.Fatal("expected purge failure")
	}
	failed, err := catalogService.Get(ctx, 1001, failedID)
	if err != nil {
		t.Fatalf("get failed purge row: %v", err)
	}
	if failed.Status != sqlcgen.FileStatusDeletionPending {
		t.Fatalf("failed purge status = %s", failed.Status)
	}
}

func TestCopyCompensatesPartialTelegramSuccess(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, "INSERT INTO channels (channel_id,user_id,name,selected) VALUES (9001,1001,'storage',true)"); err != nil {
		t.Fatal(err)
	}
	sourceID := uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,name,normalized_name,kind,mime_type,size,encryption,status,mod_time)
VALUES ($1,1001,'two-part.bin','two-part.bin','file','application/octet-stream',8,false,'active',now())`, sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO file_parts (file_id,part_no,channel_id,message_id,plain_size,stored_size,checksum,block_hashes)
VALUES
($1,1,9001,41,4,4,repeat('a',64),decode(repeat('ab',32),'hex')),
($1,2,9001,42,4,4,repeat('b',64),decode(repeat('cd',32),'hex'))`, sourceID); err != nil {
		t.Fatal(err)
	}
	storage := &fileStorage{
		messages: map[int64][]byte{41: []byte("part"), 42: []byte("part")},
		nextID:   200, failCopyAt: 2,
	}
	catalogService := catalog.NewService(db.Pool, nil)
	channelService := channels.NewService(db.Pool, nil, channels.Config{PartLimit: 100})
	service, err := NewService(db.Pool, catalogService, channelService, storage)
	if err != nil {
		t.Fatal(err)
	}
	name := "rollback-copy.bin"
	if _, err := service.Copy(ctx, CopyInput{UserID: 1001, FileID: sourceID, Name: &name}); err == nil {
		t.Fatal("expected second Telegram copy to fail")
	}
	if got := storage.message(201); got != nil {
		t.Fatalf("compensated Telegram message survived: %q", got)
	}
	if got := storage.message(41); !bytes.Equal(got, []byte("part")) {
		t.Fatalf("source message was changed: %q", got)
	}
	var count int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM files WHERE user_id=1001 AND normalized_name='rollback-copy.bin'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("copy catalog row was published after failure: %d", count)
	}
}

func TestPurgeFolderClearsUploadSessionParent(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	folderID := uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,name,normalized_name,kind,encryption,status,mod_time)
VALUES ($1,1001,'uploads','uploads','folder',false,'active',now())`, folderID); err != nil {
		t.Fatal(err)
	}
	uploadID := uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO upload_sessions (id,user_id,parent_id,name,normalized_name,expected_size,mod_time,encryption,conflict_policy,part_size,state,expires_at)
VALUES ($1,1001,$2,'pending.bin','pending.bin',1,now(),false,'fail',1,'aborted',now())`, uploadID, folderID); err != nil {
		t.Fatal(err)
	}
	catalogService := catalog.NewService(db.Pool, nil)
	service, err := NewService(db.Pool, catalogService, channels.NewService(db.Pool, nil, channels.Config{PartLimit: 100}), &fileStorage{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalogService.Trash(ctx, 1001, folderID); err != nil {
		t.Fatal(err)
	}
	if err := service.Purge(ctx, 1001, folderID); err != nil {
		t.Fatalf("Purge() error = %v", err)
	}
	var parentID *uuid.UUID
	if err := db.Pool.QueryRow(ctx, "SELECT parent_id FROM upload_sessions WHERE id = $1", uploadID).Scan(&parentID); err != nil {
		t.Fatal(err)
	}
	if parentID != nil {
		t.Fatalf("upload session parent after purge = %s", *parentID)
	}
}

func insertStoredFile(t testing.TB, db *testpostgres.Database, name string, messageID int64) uuid.UUID {
	t.Helper()
	id := uuid.New()
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,name,normalized_name,kind,mime_type,size,hash_algorithm,hash_value,encryption,status,mod_time)
VALUES ($1,1001,$2,lower($2),'file','application/octet-stream',4,'blake3',repeat('a',64),false,'active',now())`, id, name); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO file_parts (file_id,part_no,channel_id,message_id,plain_size,stored_size,checksum,block_hashes)
VALUES ($1,1,9001,$2,4,4,repeat('b',64),decode(repeat('ab',32),'hex'))`, id, messageID); err != nil {
		t.Fatal(err)
	}
	return id
}

type fileStorage struct {
	mu         sync.Mutex
	messages   map[int64][]byte
	nextID     int64
	deleteErr  error
	copyCalls  int
	failCopyAt int
}

func (*fileStorage) Upload(context.Context, telegramstore.UploadRequest) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, errors.New("not used")
}
func (s *fileStorage) OpenRange(_ context.Context, request telegramstore.RangeRequest) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, ok := s.messages[request.MessageID]
	if !ok {
		return nil, telegramstore.ErrMessageNotFound
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), payload...))), nil
}
func (s *fileStorage) DeleteMessages(_ context.Context, _ int64, _ int64, ids []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleteErr != nil {
		return s.deleteErr
	}
	for _, id := range ids {
		delete(s.messages, id)
	}
	return nil
}
func (s *fileStorage) CopyPart(_ context.Context, _ int64, _ int64, sourceMessageID, destinationChannelID int64) (telegramstore.StoredPart, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	payload, ok := s.messages[sourceMessageID]
	if !ok {
		return telegramstore.StoredPart{}, telegramstore.ErrMessageNotFound
	}
	s.copyCalls++
	if s.failCopyAt > 0 && s.copyCalls == s.failCopyAt {
		return telegramstore.StoredPart{}, errors.New("injected Telegram copy failure")
	}
	s.nextID++
	s.messages[s.nextID] = append([]byte(nil), payload...)
	return telegramstore.StoredPart{ChannelID: destinationChannelID, MessageID: s.nextID, Size: int64(len(payload))}, nil
}
func (*fileStorage) CreateChannel(context.Context, int64, string) (telegramstore.Channel, error) {
	return telegramstore.Channel{}, errors.New("not used")
}
func (*fileStorage) DeleteChannel(context.Context, int64, int64) error { return nil }
func (s *fileStorage) message(id int64) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.messages[id]...)
}
