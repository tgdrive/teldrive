package integration_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tgdrive/teldrive/internal/api"
	authpkg "github.com/tgdrive/teldrive/internal/auth"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	dbtypes "github.com/tgdrive/teldrive/internal/database/types"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/pkg/services"
)

func TestCompatService_FileMethodsCoverage(t *testing.T) {
	s := newSuite(t)
	token := loginAndGetToken(t, s, 7501, "user7501")

	sec := authpkg.NewSecurityHandler(s.repos.Sessions, s.repos.APIKeys, s.cache, &s.cfg.JWT)
	ctx, err := sec.HandleBearerAuth(context.Background(), api.OperationName("compat"), api.BearerAuth{Token: token})
	if err != nil {
		t.Fatalf("security ctx: %v", err)
	}

	cm := tgc.NewChannelManager(s.repos, s.cache, &s.cfg.TG)
	svc := services.NewApiService(s.repos, cm, s.cfg, s.cache, s.tgMock, s.events, nil)

	selected := true
	if err := s.repos.Channels.Create(ctx, &jetmodel.Channels{UserID: 7501, ChannelID: 950001, ChannelName: "selected", Selected: &selected}); err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	root, err := s.repos.Files.ResolvePathID(ctx, "/", 7501)
	if err != nil {
		t.Fatalf("resolve root: %v", err)
	}
	if root == nil {
		t.Fatalf("root folder id is nil")
	}

	srcID := uuid.New()
	status := "active"
	size := int64(5)
	cat := "document"
	if err := s.repos.Files.Create(ctx, &jetmodel.Files{ID: srcID, Name: "src.txt", Type: "file", MimeType: "text/plain", UserID: 7501, ParentID: root, Status: &status, Size: &size, Category: &cat, ChannelID: func() *int64 { v := int64(950001); return &v }(), UpdatedAt: time.Now().UTC(), CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("seed src file: %v", err)
	}

	_, _ = svc.FilesCategoryStats(ctx)

	if err := svc.FilesMkdir(ctx, "/extra/path"); err != nil {
		t.Fatalf("FilesMkdir failed: %v", err)
	}

	if _, err := svc.FilesStreamHead(ctx, api.FilesStreamHeadParams{ID: api.UUID(srcID)}); err != nil {
		t.Fatalf("FilesStreamHead failed: %v", err)
	}
	if res, err := svc.FilesStreamHead(ctx, api.FilesStreamHeadParams{ID: api.UUID(srcID), Range: api.NewOptString("bytes=1-3")}); err != nil {
		t.Fatalf("FilesStreamHead ranged failed: %v", err)
	} else if partial, ok := res.(*api.FilesStreamHeadPartialContent); !ok {
		t.Fatalf("expected partial head response, got %T", res)
	} else if got, ok := partial.ContentRange.Get(); !ok || got != "bytes 1-3/5" {
		t.Fatalf("unexpected content-range: %q", got)
	}

	if err := svc.FilesMove(ctx, &api.FileMove{Ids: []api.UUID{api.UUID(srcID)}, DestinationParent: "/extra/path", DestinationName: api.NewOptString("moved.txt")}); err != nil {
		t.Fatalf("FilesMove failed: %v", err)
	}

	copySrcID := uuid.New()
	copyParts := dbtypes.NewJSONB(dbtypes.Parts{{ID: 1}})
	if err := s.repos.Files.Create(ctx, &jetmodel.Files{ID: copySrcID, Name: "copy-src.txt", Type: "file", MimeType: "text/plain", UserID: 7501, ParentID: root, Status: &status, Parts: &copyParts, Size: &size, Category: &cat, ChannelID: func() *int64 { v := int64(950001); return &v }(), UpdatedAt: time.Now().UTC(), CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("seed copy src file: %v", err)
	}

	s.tgMock.copyFilePartsFn = func(_ context.Context, _ services.TelegramClient, _ int64, _ int64, _ []api.Part) ([]api.Part, error) {
		return []api.Part{{ID: 1}}, nil
	}
	if copied, err := svc.FilesCopy(ctx, &api.FileCopy{Destination: "/extra/path", NewName: api.NewOptString("copied.txt")}, api.FilesCopyParams{ID: api.UUID(copySrcID)}); err != nil {
		t.Fatalf("FilesCopy failed: %v", err)
	} else if copied.Name != "copied.txt" || len(copied.Parts) != 1 {
		t.Fatalf("unexpected FilesCopy result: %+v", copied)
	}
}

func TestCompatService_ShareEditDeleteCoverage(t *testing.T) {
	s := newSuite(t)
	_, client, _ := loginWithClient(t, s, 7502, "user7502")
	ctx := context.Background()

	folder, err := client.FilesCreate(ctx, &api.File{Name: "share-folder-2", Type: api.FileTypeFolder, Path: api.NewOptString("/")})
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	if err := client.FilesCreateShare(ctx, &api.FileShareCreate{}, api.FilesCreateShareParams{ID: folder.ID.Value}); err != nil {
		t.Fatalf("create share: %v", err)
	}
	shares, err := client.FilesListShares(ctx, api.FilesListSharesParams{ID: folder.ID.Value})
	if err != nil || len(shares) == 0 {
		t.Fatalf("list shares: %v len=%d", err, len(shares))
	}

	if err := client.FilesEditShare(ctx, &api.FileShareCreate{Password: api.NewOptString("newpw")}, api.FilesEditShareParams{ID: folder.ID.Value, ShareId: shares[0].ID}); err != nil {
		t.Fatalf("FilesEditShare failed: %v", err)
	}
	if err := client.FilesDeleteShare(ctx, api.FilesDeleteShareParams{ID: folder.ID.Value, ShareId: shares[0].ID}); err != nil {
		t.Fatalf("FilesDeleteShare failed: %v", err)
	}
}

func TestCompatService_StreamRoutesCoverage(t *testing.T) {
	s := newSuite(t)
	token := loginAndGetToken(t, s, 7503, "user7503")
	ctx := context.Background()

	selected := true
	if err := s.repos.Channels.Create(ctx, &jetmodel.Channels{UserID: 7503, ChannelID: 950003, ChannelName: "stream", Selected: &selected}); err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	client := s.newClientWithToken(token)
	file, err := client.FilesCreate(ctx, &api.File{Name: "empty.txt", Type: api.FileTypeFile, Path: api.NewOptString("/"), MimeType: api.NewOptString("text/plain"), ChannelId: api.NewOptInt64(950003), Size: api.NewOptInt64(0)})
	if err != nil {
		t.Fatalf("create file: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/files/%s/content", s.server.URL, uuid.UUID(file.ID.Value).String()), nil)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	req.Header.Set("Cookie", "access_token="+token)
	resp, err := s.httpCli.Do(req)
	if err != nil {
		t.Fatalf("stream do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected stream 200, got %d", resp.StatusCode)
	}

	unauthReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/events/stream", s.server.URL), nil)
	if err != nil {
		t.Fatalf("events request: %v", err)
	}
	unauthResp, err := (&http.Client{}).Do(unauthReq)
	if err != nil {
		t.Fatalf("events do: %v", err)
	}
	defer unauthResp.Body.Close()
	if unauthResp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(unauthResp.Body)
		t.Fatalf("expected events stream 401, got %d body=%s", unauthResp.StatusCode, string(body))
	}

	badShareReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/shares/%s/files/%s/content", s.server.URL, "bad", uuid.UUID(file.ID.Value).String()), nil)
	if err != nil {
		t.Fatalf("bad share request: %v", err)
	}
	badShareResp, err := s.httpCli.Do(badShareReq)
	if err != nil {
		t.Fatalf("bad share do: %v", err)
	}
	defer badShareResp.Body.Close()
	if badShareResp.StatusCode != http.StatusUnauthorized && badShareResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected unauthorized/bad request for share stream, got %d", badShareResp.StatusCode)
	}

	if !strings.Contains(resp.Header.Get("Accept-Ranges"), "bytes") {
		t.Fatalf("expected Accept-Ranges header")
	}
}
