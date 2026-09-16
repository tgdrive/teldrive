//go:build integration

package fileops

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/channels"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"github.com/tgdrive/teldrive/v2/internal/testutil/querytrace"
)

func TestCopyWideFolderUsesSetBasedCatalogQueries(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, "INSERT INTO channels (channel_id,user_id,name,selected) VALUES (9001,1001,'storage',true)"); err != nil {
		t.Fatal(err)
	}
	sourceID, destinationID := uuid.New(), uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,name,normalized_name,kind,encryption,status,mod_time)
VALUES ($1,1001,'source','source','folder',false,'active',now()),
       ($2,1001,'destination','destination','folder',false,'active',now())`, sourceID, destinationID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
WITH children AS (
  INSERT INTO files (user_id,parent_id,name,normalized_name,kind,size,encryption,status,mod_time)
  SELECT 1001, $1, 'child-' || value, 'child-' || value, 'file', 1, false, 'active', now()
  FROM generate_series(1, 1000) AS value
  RETURNING id
)
INSERT INTO file_parts (file_id,part_no,channel_id,message_id,plain_size,stored_size,checksum,block_hashes)
SELECT id, 1, 9001, row_number() OVER ()::bigint, 1, 1, repeat('a',64), decode(repeat('ab',32),'hex')
FROM children`, sourceID); err != nil {
		t.Fatal(err)
	}
	messages := make(map[int64][]byte, 1000)
	for id := int64(1); id <= 1000; id++ {
		messages[id] = []byte{'x'}
	}
	storage := &fileStorage{messages: messages, nextID: 1000}
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
	service, err := NewService(pool, catalog.NewService(pool, nil), channels.NewService(pool, nil, channels.Config{PartLimit: 2000}), storage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Copy(ctx, CopyInput{UserID: 1001, FileID: sourceID, ParentID: &destinationID}); err != nil {
		t.Fatalf("Copy() error = %v", err)
	}
	for _, name := range []string{"ListFilePartsByFileIDs", "GetSelectedChannel", "CountChannelStoredMessages", "InsertCopiedFiles", "InsertCopiedFileParts"} {
		if got := tracer.Count(name); got != 1 {
			t.Fatalf("%s queries = %d, want 1", name, got)
		}
	}
	for _, name := range []string{"ListFileParts", "InsertCopiedFile", "InsertCopiedFilePart"} {
		if got := tracer.Count(name); got != 0 {
			t.Fatalf("%s queries = %d, want 0", name, got)
		}
	}
}

func TestCleanTrashMarksAllUserTrashDeletionPending(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001), (1002)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, "INSERT INTO channels (channel_id,user_id,name,selected) VALUES (9001,1001,'storage',true)"); err != nil {
		t.Fatal(err)
	}
	trashedA, trashedB, active, otherUser := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,name,normalized_name,kind,size,encryption,status,mod_time,deleted_at)
VALUES
($1,1001,'a','a','file',0,false,'trashed',now(),now()),
($2,1001,'b','b','folder',NULL,false,'trashed',now(),now()),
($3,1001,'active','active','file',0,false,'active',now(),NULL),
($4,1002,'other','other','file',0,false,'trashed',now(),now())`, trashedA, trashedB, active, otherUser); err != nil {
		t.Fatal(err)
	}
	catalogService := catalog.NewService(db.Pool, nil)
	channelService := channels.NewService(db.Pool, nil, channels.Config{PartLimit: 100})
	service, err := NewService(db.Pool, catalogService, channelService, &fileStorage{messages: map[int64][]byte{}})
	if err != nil {
		t.Fatal(err)
	}
	count, err := service.CleanTrash(ctx, 1001)
	if err != nil {
		t.Fatalf("CleanTrash() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("CleanTrash() count = %d, want 2", count)
	}
	for _, tc := range []struct {
		id   uuid.UUID
		want string
	}{
		{trashedA, "deletion_pending"},
		{trashedB, "deletion_pending"},
		{active, "active"},
		{otherUser, "trashed"},
	} {
		var status string
		if err := db.Pool.QueryRow(ctx, "SELECT status::text FROM files WHERE id = $1", tc.id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != tc.want {
			t.Fatalf("file %s status = %s, want %s", tc.id, status, tc.want)
		}
	}
}

func TestQueuePurgeMarksSubtreeWithoutDeletingTelegramMessages(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	rootID, childID := uuid.New(), uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,parent_id,name,normalized_name,kind,size,encryption,status,mod_time,deleted_at)
VALUES
($1,1001,NULL,'folder','folder','folder',NULL,false,'trashed',now(),now()),
($2,1001,$1,'child','child','file',1,false,'trashed',now(),now())`, rootID, childID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO file_parts (file_id,part_no,channel_id,message_id,plain_size,stored_size,checksum,block_hashes)
VALUES ($1,1,9001,10,1,1,repeat('a',64),decode(repeat('ab',32),'hex'))`, childID); err != nil {
		t.Fatal(err)
	}
	storage := &fileStorage{messages: map[int64][]byte{10: []byte("data")}}
	service, err := NewService(db.Pool, catalog.NewService(db.Pool, nil), channels.NewService(db.Pool, nil, channels.Config{PartLimit: 100}), storage)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.QueuePurge(ctx, 1001, rootID); err != nil {
		t.Fatalf("QueuePurge() error = %v", err)
	}
	for _, id := range []uuid.UUID{rootID, childID} {
		var status sqlcgen.FileStatus
		if err := db.Pool.QueryRow(ctx, "SELECT status FROM files WHERE id = $1", id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != sqlcgen.FileStatusDeletionPending {
			t.Fatalf("file %s status = %s", id, status)
		}
	}
	if calls := storage.deleteCallsSnapshot(); len(calls) != 0 {
		t.Fatalf("DeleteMessages() calls = %d, want 0", len(calls))
	}
	if got := storage.message(10); !bytes.Equal(got, []byte("data")) {
		t.Fatalf("queued Telegram message = %q", got)
	}
}

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
	storage.mu.Lock()
	storage.deleteErr = nil
	storage.mu.Unlock()
	if err := service.Purge(ctx, 1001, failedID); err != nil {
		t.Fatalf("retry Purge() error = %v", err)
	}
	if _, err := catalogService.Get(ctx, 1001, failedID); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("get retried purge row error = %v, want not found", err)
	}
}

func TestPurgeManyGroupsTelegramMessagesByChannel(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, "INSERT INTO channels (channel_id,user_id,name,selected) VALUES (9001,1001,'storage',true)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
WITH files AS (
    INSERT INTO files (user_id,name,normalized_name,kind,size,encryption,status,mod_time,deleted_at)
    SELECT 1001, 'pending-' || value, 'pending-' || value, 'file', 1, false, 'deletion_pending', now(), now()
    FROM generate_series(1, 1000) AS value
    RETURNING id
)
INSERT INTO file_parts (file_id,part_no,channel_id,message_id,plain_size,stored_size,checksum,block_hashes)
SELECT id, 1, 9001, row_number() OVER ()::bigint, 1, 1, repeat('b',64), decode(repeat('ab',32),'hex')
FROM files
`); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Pool.Query(ctx, "SELECT id FROM files WHERE user_id = 1001 AND status = 'deletion_pending'")
	if err != nil {
		t.Fatal(err)
	}
	var fileIDs []uuid.UUID
	for rows.Next() {
		var fileID uuid.UUID
		if err := rows.Scan(&fileID); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		fileIDs = append(fileIDs, fileID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	storage := &fileStorage{}
	service, err := NewService(db.Pool, catalog.NewService(db.Pool, nil), channels.NewService(db.Pool, nil, channels.Config{PartLimit: 100}), storage)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.PurgeMany(ctx, 1001, fileIDs); err != nil {
		t.Fatalf("PurgeMany() error = %v", err)
	}
	deleteCalls := storage.deleteCallsSnapshot()
	if len(deleteCalls) != 1 || len(deleteCalls[0]) != 1000 {
		t.Fatalf("DeleteMessages() calls = %d with sizes %v, want one call of 1000", len(deleteCalls), sliceLengths(deleteCalls))
	}
	var remaining int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM files WHERE user_id = 1001").Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("remaining files = %d, want 0", remaining)
	}
}

func TestPurgeWideFolderUsesDepthBatchedQueries(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, "INSERT INTO channels (channel_id,user_id,name,selected) VALUES (9001,1001,'storage',true)"); err != nil {
		t.Fatal(err)
	}
	rootID := uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,name,normalized_name,kind,encryption,status,mod_time,deleted_at)
VALUES ($1,1001,'folder','folder','folder',false,'trashed',now(),now())`, rootID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
WITH children AS (
    INSERT INTO files (user_id,parent_id,name,normalized_name,kind,size,encryption,status,mod_time,deleted_at)
    SELECT 1001, $1, 'child-' || value, 'child-' || value, 'file', 1, false, 'trashed', now(), now()
    FROM generate_series(1, 1000) AS value
    RETURNING id
)
INSERT INTO file_parts (file_id,part_no,channel_id,message_id,plain_size,stored_size,checksum,block_hashes)
SELECT id, 1, 9001, row_number() OVER ()::bigint, 1, 1, repeat('b',64), decode(repeat('ab',32),'hex')
FROM children`, rootID); err != nil {
		t.Fatal(err)
	}

	tracer := &purgeQueryTracer{}
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
	service, err := NewService(pool, catalog.NewService(pool, nil), channels.NewService(pool, nil, channels.Config{PartLimit: 100}), &fileStorage{})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Purge(ctx, 1001, rootID); err != nil {
		t.Fatalf("Purge() error = %v", err)
	}
	if got := tracer.count("TryAdvisoryLocks"); got != 1 {
		t.Fatalf("TryAdvisoryLocks queries = %d, want 1", got)
	}
	if got := tracer.count("LoadFileSubtrees"); got != 1 {
		t.Fatalf("LoadFileSubtrees queries = %d, want 1", got)
	}
	if got := tracer.count("DeleteFileCatalogRowsByIDs"); got != 2 {
		t.Fatalf("DeleteFileCatalogRowsByIDs queries = %d, want 2", got)
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
	mu          sync.Mutex
	messages    map[int64][]byte
	nextID      int64
	deleteErr   error
	copyCalls   int
	failCopyAt  int
	deleteCalls [][]int64
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
	s.deleteCalls = append(s.deleteCalls, append([]int64(nil), ids...))
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

func (s *fileStorage) deleteCallsSnapshot() [][]int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([][]int64, len(s.deleteCalls))
	for i, ids := range s.deleteCalls {
		result[i] = append([]int64(nil), ids...)
	}
	return result
}

func sliceLengths(values [][]int64) []int {
	lengths := make([]int, len(values))
	for i, value := range values {
		lengths[i] = len(value)
	}
	return lengths
}

type purgeQueryTracer struct {
	mu     sync.Mutex
	counts map[string]int
}

func (t *purgeQueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	for _, name := range []string{"TryAdvisoryLocks", "LoadFileSubtrees", "DeleteFileCatalogRowsByIDs"} {
		if strings.Contains(data.SQL, "-- name: "+name) {
			t.mu.Lock()
			if t.counts == nil {
				t.counts = make(map[string]int)
			}
			t.counts[name]++
			t.mu.Unlock()
		}
	}
	return ctx
}

func (*purgeQueryTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (t *purgeQueryTracer) count(name string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.counts[name]
}
