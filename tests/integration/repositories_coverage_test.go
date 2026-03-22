package integration_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jetgen/teldrive_jet/teldrive/model"
	"github.com/tgdrive/teldrive/pkg/repositories"
)

func TestRepositories_CoveragePaths(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()

	userRepo := repositories.NewJetUserRepository(s.pool)
	botRepo := repositories.NewJetBotRepository(s.pool)
	sessionRepo := repositories.NewJetSessionRepository(s.pool)
	channelRepo := repositories.NewJetChannelRepository(s.pool)
	eventRepo := repositories.NewJetEventRepository(s.pool)
	fileRepo := repositories.NewJetFileRepository(s.pool)
	kvRepo := repositories.NewJetKVRepository(s.pool)
	shareRepo := repositories.NewJetShareRepository(s.pool)
	uploadRepo := repositories.NewJetUploadRepository(s.pool)

	uid := int64(7401)
	now := time.Now().UTC()
	name := "repo-user"
	if err := userRepo.Create(ctx, &jetmodel.Users{UserID: uid, UserName: "repo_user", Name: &name, IsPremium: false, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("user create: %v", err)
	}
	if _, err := userRepo.GetByID(ctx, uid); err != nil {
		t.Fatalf("user get: %v", err)
	}
	newName := "repo-user-updated"
	newUsername := "repo_user_updated"
	premium := true
	if err := userRepo.Update(ctx, uid, repositories.UserUpdate{Name: &newName, UserName: &newUsername, IsPremium: &premium}); err != nil {
		t.Fatalf("user update: %v", err)
	}
	if _, err := userRepo.Exists(ctx, uid); err != nil {
		t.Fatalf("user exists: %v", err)
	}
	if _, err := userRepo.All(ctx); err != nil {
		t.Fatalf("user all: %v", err)
	}

	selected := true
	if err := channelRepo.Create(ctx, &jetmodel.Channels{UserID: uid, ChannelID: 930001, ChannelName: "c1", Selected: &selected}); err != nil {
		t.Fatalf("channel create: %v", err)
	}
	if err := channelRepo.Create(ctx, &jetmodel.Channels{UserID: uid, ChannelID: 930002, ChannelName: "c2"}); err != nil {
		t.Fatalf("channel create2: %v", err)
	}
	if _, err := channelRepo.GetByUserID(ctx, uid); err != nil {
		t.Fatalf("channels by user: %v", err)
	}
	if _, err := channelRepo.GetByChannelID(ctx, 930001); err != nil {
		t.Fatalf("channel by id: %v", err)
	}
	if _, err := channelRepo.GetSelected(ctx, uid); err != nil {
		t.Fatalf("channel selected: %v", err)
	}
	selectedFalse := false
	channelName := "c1-updated"
	if err := channelRepo.Update(ctx, 930001, repositories.ChannelUpdate{Selected: &selectedFalse, ChannelName: &channelName}); err != nil {
		t.Fatalf("channel update: %v", err)
	}
	if err := s.repos.WithTx(ctx, func(txCtx context.Context) error {
		return s.repos.Channels.Create(txCtx, &jetmodel.Channels{UserID: uid, ChannelID: 930003, ChannelName: "c3"})
	}); err != nil {
		t.Fatalf("channel tx: %v", err)
	}
	if err := channelRepo.Delete(ctx, 930003); err != nil {
		t.Fatalf("channel delete: %v", err)
	}

	if err := botRepo.Create(ctx, &jetmodel.Bots{UserID: uid, Token: "111:aaa", BotID: 111}); err != nil {
		t.Fatalf("bot create: %v", err)
	}
	if err := botRepo.Create(ctx, &jetmodel.Bots{UserID: uid, Token: "222:bbb", BotID: 222}); err != nil {
		t.Fatalf("bot create2: %v", err)
	}
	if _, err := botRepo.GetByUserID(ctx, uid); err != nil {
		t.Fatalf("bots by user: %v", err)
	}
	if _, err := botRepo.GetTokensByUserID(ctx, uid); err != nil {
		t.Fatalf("bot tokens: %v", err)
	}
	if err := botRepo.Delete(ctx, uid, "111:aaa"); err != nil {
		t.Fatalf("bot delete: %v", err)
	}

	created := now
	updated := now
	sid1 := uuid.New()
	sid2 := uuid.New()
	if err := sessionRepo.Create(ctx, &jetmodel.Sessions{ID: sid1, UserID: uid, TgSession: "s1", CreatedAt: created, UpdatedAt: updated}); err != nil {
		t.Fatalf("session create: %v", err)
	}
	if err := sessionRepo.Create(ctx, &jetmodel.Sessions{ID: sid2, UserID: uid, TgSession: "s2", CreatedAt: created, UpdatedAt: updated}); err != nil {
		t.Fatalf("session create2: %v", err)
	}
	if _, err := sessionRepo.GetByID(ctx, sid1.String()); err != nil {
		t.Fatalf("session by hash: %v", err)
	}
	rh := "rh1"
	if err := sessionRepo.UpdateRefreshTokenHash(ctx, sid1.String(), rh); err != nil {
		t.Fatalf("session update refresh hash: %v", err)
	}
	if _, err := sessionRepo.GetByRefreshTokenHash(ctx, rh); err != nil {
		t.Fatalf("session by refresh hash: %v", err)
	}
	if _, err := sessionRepo.GetByUserID(ctx, uid); err != nil {
		t.Fatalf("sessions by user: %v", err)
	}
	if err := sessionRepo.Revoke(ctx, sid2.String()); err != nil {
		t.Fatalf("session revoke: %v", err)
	}

	if err := kvRepo.Set(ctx, &jetmodel.Kv{Key: "peer:a", Value: []byte("a")}); err != nil {
		t.Fatalf("kv set: %v", err)
	}
	if err := kvRepo.Set(ctx, &jetmodel.Kv{Key: "peer:b", Value: []byte("b")}); err != nil {
		t.Fatalf("kv set2: %v", err)
	}
	if _, err := kvRepo.Get(ctx, "peer:a"); err != nil {
		t.Fatalf("kv get: %v", err)
	}
	iterCount := 0
	if err := kvRepo.Iterate(ctx, "peer:", func(key string, _ []byte) error {
		iterCount++
		if !strings.HasPrefix(key, "peer:") {
			return fmt.Errorf("unexpected key %s", key)
		}
		return nil
	}); err != nil {
		t.Fatalf("kv iterate: %v", err)
	}
	if iterCount < 2 {
		t.Fatalf("expected >=2 iter entries, got %d", iterCount)
	}
	if err := kvRepo.Delete(ctx, "peer:a"); err != nil {
		t.Fatalf("kv delete: %v", err)
	}
	if err := kvRepo.DeletePrefix(ctx, "peer:"); err != nil {
		t.Fatalf("kv delete prefix: %v", err)
	}

	rootID, err := fileRepo.CreateDirectories(ctx, uid, "/docs/sub")
	if err != nil {
		t.Fatalf("create directories: %v", err)
	}
	resolved, err := fileRepo.ResolvePathID(ctx, "/docs/sub", uid)
	if err != nil || resolved == nil || *resolved != *rootID {
		t.Fatalf("resolve path: id=%v err=%v", resolved, err)
	}

	active := "active"
	fileAID := uuid.New()
	fileBID := uuid.New()
	sizeA := int64(10)
	sizeB := int64(20)
	catDoc := "document"
	chID := int64(930001)
	if err := fileRepo.Create(ctx, &jetmodel.Files{ID: fileAID, Name: "a.txt", Type: "file", MimeType: "text/plain", UserID: uid, ParentID: rootID, Status: &active, Size: &sizeA, Category: &catDoc, ChannelID: &chID, Encrypted: false, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("file create a: %v", err)
	}
	if err := fileRepo.Create(ctx, &jetmodel.Files{ID: fileBID, Name: "b.txt", Type: "file", MimeType: "text/plain", UserID: uid, ParentID: rootID, Status: &active, Size: &sizeB, Category: &catDoc, ChannelID: &chID, Encrypted: false, CreatedAt: now.Add(-25 * time.Hour), UpdatedAt: now.Add(-25 * time.Hour)}); err != nil {
		t.Fatalf("file create b: %v", err)
	}
	if _, err := fileRepo.GetByIDAndUser(ctx, fileAID, uid); err != nil {
		t.Fatalf("file by id user: %v", err)
	}
	if _, err := fileRepo.GetByChannelID(ctx, chID); err != nil {
		t.Fatalf("files by channel: %v", err)
	}
	if _, err := fileRepo.GetFullPath(ctx, fileAID); err != nil {
		t.Fatalf("full path: %v", err)
	}
	_, _ = fileRepo.CategoryStats(ctx, uid)

	cursor := now.Format(time.RFC3339Nano) + ":" + fileAID.String()
	if _, err := fileRepo.List(ctx, repositories.FileQueryParams{UserID: uid, Operation: "find", Status: "active", Query: "a", SearchType: "normal", Sort: "updated_at", Order: "desc", Cursor: cursor, Limit: 10, UpdatedAt: "gte:2020-01-01,lte:2100-01-01", Category: []string{"document"}}); err != nil {
		t.Fatalf("list files find: %v", err)
	}
	if _, err := fileRepo.List(ctx, repositories.FileQueryParams{UserID: uid, Operation: "list", Status: "active", Path: "/docs/sub", Sort: "name", Order: "asc", Limit: 10}); err != nil {
		t.Fatalf("list files list path: %v", err)
	}

	nameMoved := "a-moved.txt"
	if err := fileRepo.MoveSingle(ctx, fileAID, uid, rootID, &nameMoved); err != nil {
		t.Fatalf("move single: %v", err)
	}
	if err := fileRepo.MoveBulk(ctx, []uuid.UUID{fileBID}, uid, rootID); err != nil {
		t.Fatalf("move bulk: %v", err)
	}

	folderID := uuid.New()
	if err := fileRepo.Create(ctx, &jetmodel.Files{ID: folderID, Name: "folder-x", Type: "folder", MimeType: "drive/folder", UserID: uid, ParentID: rootID, Status: &active, Encrypted: false, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("folder create: %v", err)
	}
	childID := uuid.New()
	if err := fileRepo.Create(ctx, &jetmodel.Files{ID: childID, Name: "child.txt", Type: "file", MimeType: "text/plain", UserID: uid, ParentID: &folderID, Status: &active, Size: &sizeA, Category: &catDoc, Encrypted: false, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("child file create: %v", err)
	}
	if err := fileRepo.DeleteBulk(ctx, []uuid.UUID{folderID}, uid, "trashed"); err != nil {
		t.Fatalf("delete bulk: %v", err)
	}
	trashedFolder, err := fileRepo.GetByID(ctx, folderID)
	if err != nil {
		t.Fatalf("get trashed folder: %v", err)
	}
	if trashedFolder.Status == nil || *trashedFolder.Status != "trashed" {
		t.Fatalf("expected folder status trashed, got %+v", trashedFolder.Status)
	}
	trashedChild, err := fileRepo.GetByID(ctx, childID)
	if err != nil {
		t.Fatalf("get trashed child: %v", err)
	}
	if trashedChild.Status == nil || *trashedChild.Status != "trashed" {
		t.Fatalf("expected child status trashed, got %+v", trashedChild.Status)
	}

	if err := s.repos.WithTx(ctx, func(ctx context.Context) error {
		fid := uuid.New()
		return s.repos.Files.Create(ctx, &jetmodel.Files{ID: fid, Name: "tx.txt", Type: "file", MimeType: "text/plain", UserID: uid, ParentID: rootID, Status: &active, Size: &sizeA, Category: &catDoc, Encrypted: false, CreatedAt: now, UpdatedAt: now})
	}); err != nil {
		t.Fatalf("file tx: %v", err)
	}

	shareID := uuid.New()
	password := "pw"
	if err := shareRepo.Create(ctx, &jetmodel.FileShares{ID: shareID, FileID: fileAID, UserID: uid, Password: &password, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("share create: %v", err)
	}
	if _, err := shareRepo.GetByFileID(ctx, fileAID); err != nil {
		t.Fatalf("share by file: %v", err)
	}
	if _, err := shareRepo.GetByID(ctx, shareID); err != nil {
		t.Fatalf("share by id: %v", err)
	}
	np := "newpw"
	exp := now.Add(24 * time.Hour)
	if err := shareRepo.Update(ctx, shareID, repositories.ShareUpdate{Password: &np, ExpiresAt: &exp}); err != nil {
		t.Fatalf("share update: %v", err)
	}
	if err := shareRepo.DeleteByFileID(ctx, fileAID); err != nil {
		t.Fatalf("share delete by file: %v", err)
	}
	if err := shareRepo.Create(ctx, &jetmodel.FileShares{ID: shareID, FileID: fileAID, UserID: uid, Password: &password, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("share recreate: %v", err)
	}
	if err := shareRepo.Delete(ctx, shareID); err != nil {
		t.Fatalf("share delete: %v", err)
	}

	if err := eventRepo.Create(ctx, &jetmodel.Events{ID: uuid.New(), Type: "upload", UserID: uid, CreatedAt: now.Add(-2 * time.Hour)}); err != nil {
		t.Fatalf("event create old: %v", err)
	}
	if err := eventRepo.Create(ctx, &jetmodel.Events{ID: uuid.New(), Type: "delete", UserID: uid, CreatedAt: now}); err != nil {
		t.Fatalf("event create new: %v", err)
	}
	if _, err := eventRepo.GetByUserID(ctx, uid, now.Add(-24*time.Hour)); err != nil {
		t.Fatalf("events by user: %v", err)
	}
	if _, err := eventRepo.GetRecent(ctx, uid, now.Add(-24*time.Hour), 1); err != nil {
		t.Fatalf("events recent: %v", err)
	}
	if _, err := eventRepo.GetSince(ctx, now.Add(-24*time.Hour), 10); err != nil {
		t.Fatalf("events since: %v", err)
	}
	if _, err := eventRepo.DeleteOlderThan(ctx, now.Add(-time.Hour)); err != nil {
		t.Fatalf("events delete older: %v", err)
	}

	oldCreated := now.Add(-2 * time.Hour)
	if err := uploadRepo.Create(ctx, &jetmodel.Uploads{UploadID: "up-ret", Name: "p1", UserID: &uid, PartNo: 1, PartID: 15001, ChannelID: chID, Size: 5, CreatedAt: &oldCreated}); err != nil {
		t.Fatalf("upload create old: %v", err)
	}
	newCreated := now
	if err := uploadRepo.Create(ctx, &jetmodel.Uploads{UploadID: "up-ret", Name: "p2", UserID: &uid, PartNo: 2, PartID: 15002, ChannelID: chID, Size: 6, CreatedAt: &newCreated}); err != nil {
		t.Fatalf("upload create new: %v", err)
	}
	if _, err := uploadRepo.GetByUploadID(ctx, "up-ret"); err != nil {
		t.Fatalf("uploads by id: %v", err)
	}
	if _, err := uploadRepo.GetByUploadIDAndRetention(ctx, "up-ret", time.Hour); err != nil {
		t.Fatalf("uploads by retention: %v", err)
	}
	if err := uploadRepo.DeleteByID(ctx, 15001, chID); err != nil {
		t.Fatalf("upload delete by id: %v", err)
	}
	if _, err := uploadRepo.StatsByDays(ctx, uid, 3); err != nil {
		t.Fatalf("upload stats: %v", err)
	}
	if err := uploadRepo.Delete(ctx, "up-ret"); err != nil {
		t.Fatalf("upload delete: %v", err)
	}

	if err := channelRepo.DeleteByUserID(ctx, uid); err != nil {
		t.Fatalf("channel delete by user: %v", err)
	}
	if err := sessionRepo.DeleteByUserID(ctx, uid); err != nil {
		t.Fatalf("session delete by user: %v", err)
	}
	if err := botRepo.DeleteByUserID(ctx, uid); err != nil {
		t.Fatalf("bot delete by user: %v", err)
	}
}
