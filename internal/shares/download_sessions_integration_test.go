//go:build integration

package shares

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestDownloadSessions(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	rootID, childID, outsideID := uuid.New(), uuid.New(), uuid.New()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
 INSERT INTO files (id,user_id,name,kind,encryption,status,mod_time)
 VALUES ($1,1001,'folder','folder',false,'active',now())`, rootID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
 INSERT INTO files (id,user_id,parent_id,name,kind,size,encryption,status,mod_time)
 VALUES ($1,1001,$2,'child.bin','file',4,false,'active',now()),
        ($3,1001,NULL,'outside.bin','file',4,false,'active',now())`, childID, rootID, outsideID); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(db.Pool, catalog.NewService(db.Pool, nil))
	if err != nil {
		t.Fatal(err)
	}
	password := "download-password"
	create := func(t *testing.T, expires *time.Time, limit *int64) (*Created, *DownloadSession) {
		t.Helper()
		share, err := service.Create(ctx, CreateInput{OwnerID: 1001, FileID: rootID, Password: &password, Permission: sqlcgen.SharePermissionEdit, ExpiresAt: expires, MaxDownloads: limit})
		if err != nil {
			t.Fatal(err)
		}
		session, err := service.CreateDownloadSession(ctx, share.Token, password)
		if err != nil {
			t.Fatal(err)
		}
		return share, session
	}

	t.Run("credentials and scope", func(t *testing.T) {
		share, session := create(t, nil, nil)
		for _, invalid := range []string{"", "wrong"} {
			if _, err := service.CreateDownloadSession(ctx, share.Token, invalid); err == nil {
				t.Fatal("invalid password authorized a session")
			}
		}
		var stored []byte
		if err := db.Pool.QueryRow(ctx, "SELECT token_hash FROM share_download_sessions WHERE token_hash=$1", tokenHash(session.Token)).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(stored, tokenHash(session.Token)) || bytes.Equal(stored, []byte(session.Token)) {
			t.Fatal("session is not stored as a digest")
		}
		if ttl := time.Until(session.ExpiresAt); ttl <= 14*time.Minute || ttl > DownloadSessionTTL {
			t.Fatalf("TTL = %s", ttl)
		}
		in := ContentRequest{Token: share.Token, Session: session.Token, FileID: &childID}
		if _, err := service.ResolveContent(ctx, in); err != nil {
			t.Fatalf("child download: %v", err)
		}
		in.FileID = &outsideID
		if _, err := service.ResolveContent(ctx, in); !errors.Is(err, ErrNotFound) {
			t.Fatalf("outside file: %v", err)
		}
		in.FileID = nil
		other, _ := create(t, nil, nil)
		in.Token = other.Token
		if _, err := service.ResolveContent(ctx, in); !errors.Is(err, ErrPasswordNeeded) {
			t.Fatalf("cross-share session: %v", err)
		}
		in.Token, in.Session = share.Token, session.Token+"tampered"
		if _, err := service.ResolveContent(ctx, in); !errors.Is(err, ErrPasswordNeeded) {
			t.Fatalf("tampered session: %v", err)
		}
		in.Session, in.Password = session.Token, "wrong"
		if _, err := service.ResolveContent(ctx, in); !errors.Is(err, ErrInvalidPassword) {
			t.Fatalf("explicit wrong password: %v", err)
		}
		// A second browser must not invalidate the first browser's authorization.
		if _, err := service.CreateDownloadSession(ctx, share.Token, password); err != nil {
			t.Fatal(err)
		}
		in.Password = ""
		if _, err := service.ResolveContent(ctx, in); err != nil {
			t.Fatalf("first browser: %v", err)
		}
		if _, err := service.ResolvePublicEditableFile(ctx, share.Token, "", childID); !errors.Is(err, ErrPasswordNeeded) {
			t.Fatalf("edit without password: %v", err)
		}
		var count int64
		if err := db.Pool.QueryRow(ctx, "SELECT download_count FROM file_shares WHERE id=$1", share.Row.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("authorization/resolve consumed %d downloads", count)
		}
	})

	t.Run("session expiry and cleanup", func(t *testing.T) {
		share, session := create(t, nil, nil)
		if _, err := db.Pool.Exec(ctx, "UPDATE share_download_sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1", tokenHash(session.Token)); err != nil {
			t.Fatal(err)
		}
		if _, err := service.ResolveContent(ctx, ContentRequest{Token: share.Token, Session: session.Token}); !errors.Is(err, ErrPasswordNeeded) {
			t.Fatalf("expired session: %v", err)
		}
		if _, err := service.CreateDownloadSession(ctx, share.Token, password); err != nil {
			t.Fatal(err)
		}
		var exists bool
		if err := db.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM share_download_sessions WHERE token_hash=$1)", tokenHash(session.Token)).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Fatal("expired session was not cleaned up")
		}
	})

	t.Run("password changes invalidate sessions", func(t *testing.T) {
		share, session := create(t, nil, nil)
		replacement := "replacement-password"
		if _, err := service.Update(ctx, UpdateInput{OwnerID: 1001, ShareID: uuid.UUID(share.Row.ID.Bytes), Password: &replacement}); err != nil {
			t.Fatal(err)
		}
		if _, err := service.ResolveContent(ctx, ContentRequest{Token: share.Token, Session: session.Token}); !errors.Is(err, ErrPasswordNeeded) {
			t.Fatalf("old password session: %v", err)
		}
	})

	t.Run("revocation", func(t *testing.T) {
		share, session := create(t, nil, nil)
		if err := service.Revoke(ctx, 1001, uuid.UUID(share.Row.ID.Bytes)); err != nil {
			t.Fatal(err)
		}
		if _, err := service.ResolveContent(ctx, ContentRequest{Token: share.Token, Session: session.Token}); !errors.Is(err, ErrExpired) {
			t.Fatalf("revoked share: %v", err)
		}
	})

	t.Run("share expiry caps session expiry", func(t *testing.T) {
		expiry := time.Now().UTC().Add(time.Minute)
		share, session := create(t, &expiry, nil)
		if session.ExpiresAt.Sub(expiry).Abs() > time.Microsecond {
			t.Fatalf("expiry %s exceeds share expiry %s", session.ExpiresAt, expiry)
		}
		if _, err := db.Pool.Exec(ctx, "UPDATE file_shares SET expires_at=now()-interval '1 second' WHERE id=$1", share.Row.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := service.ResolveContent(ctx, ContentRequest{Token: share.Token, Session: session.Token}); !errors.Is(err, ErrExpired) {
			t.Fatalf("expired share: %v", err)
		}
	})

	t.Run("download limit", func(t *testing.T) {
		limit := int64(1)
		share, session := create(t, nil, &limit)
		in := ContentRequest{Token: share.Token, Session: session.Token, FileID: &childID}
		if _, err := service.ReserveContentDownload(ctx, in); err != nil {
			t.Fatal(err)
		}
		if _, err := service.ReserveContentDownload(ctx, in); !errors.Is(err, ErrExpired) {
			t.Fatalf("exhausted share: %v", err)
		}
		if _, err := service.CreateDownloadSession(ctx, share.Token, password); !errors.Is(err, ErrExpired) {
			t.Fatalf("session for exhausted share: %v", err)
		}
	})

	t.Run("concurrent cookie downloads respect the limit", func(t *testing.T) {
		limit := int64(3)
		share, session := create(t, nil, &limit)
		in := ContentRequest{Token: share.Token, Session: session.Token, FileID: &childID}
		start := make(chan struct{})
		results := make(chan error, 12)
		for range cap(results) {
			go func() {
				<-start
				_, err := service.ReserveContentDownload(ctx, in)
				results <- err
			}()
		}
		close(start)
		var accepted int64
		for range cap(results) {
			if err := <-results; err == nil {
				accepted++
			} else if !errors.Is(err, ErrExpired) {
				t.Errorf("concurrent reservation: %v", err)
			}
		}
		if accepted != limit {
			t.Fatalf("accepted %d downloads, want %d", accepted, limit)
		}
	})
}
