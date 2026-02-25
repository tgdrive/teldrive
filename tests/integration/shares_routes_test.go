package integration_test

import (
	"context"
	"testing"

	"github.com/tgdrive/teldrive/internal/api"
)

func TestSharesRoutes_Flow(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	public, client, _ := loginWithClient(t, s, 7204, "user7204")
	if err := client.UsersUpdateChannel(ctx, &api.ChannelUpdate{ChannelId: api.NewOptInt64(910080), ChannelName: api.NewOptString("share-default")}); err != nil {
		t.Fatalf("UsersUpdateChannel failed: %v", err)
	}

	folder, err := client.FilesCreate(ctx, &api.File{Name: "share-folder", Type: api.FileTypeFolder, Path: api.NewOptString("/")})
	if err != nil {
		t.Fatalf("FilesCreate folder failed: %v", err)
	}

	file, err := client.FilesCreate(ctx, &api.File{Name: "share-file.txt", Type: api.FileTypeFile, Path: api.NewOptString("/"), MimeType: api.NewOptString("text/plain"), ChannelId: api.NewOptInt64(910080), Size: api.NewOptInt64(12)})
	if err != nil {
		t.Fatalf("FilesCreate file failed: %v", err)
	}

	if err := client.FilesCreateShare(ctx, &api.FileShareCreate{}, api.FilesCreateShareParams{ID: folder.ID.Value}); err != nil {
		t.Fatalf("FilesCreateShare folder failed: %v", err)
	}
	if err := client.FilesCreateShare(ctx, &api.FileShareCreate{}, api.FilesCreateShareParams{ID: file.ID.Value}); err != nil {
		t.Fatalf("FilesCreateShare file failed: %v", err)
	}

	folderShares, err := client.FilesListShares(ctx, api.FilesListSharesParams{ID: folder.ID.Value})
	if err != nil || len(folderShares) == 0 {
		t.Fatalf("FilesListShares folder failed: %v len=%d", err, len(folderShares))
	}

	if _, err := public.SharesGetById(ctx, api.SharesGetByIdParams{ID: folderShares[0].ID}); err != nil {
		t.Fatalf("SharesGetById failed: %v", err)
	}

	fileShares, err := client.FilesListShares(ctx, api.FilesListSharesParams{ID: file.ID.Value})
	if err != nil || len(fileShares) == 0 {
		t.Fatalf("FilesListShares file failed: %v len=%d", err, len(fileShares))
	}

	shareFiles, err := public.SharesListFiles(ctx, api.SharesListFilesParams{ID: fileShares[0].ID, Limit: api.NewOptInt(20), Sort: api.NewOptShareQuerySort(api.ShareQuerySortName), Order: api.NewOptShareQueryOrder(api.ShareQueryOrderAsc)})
	if err != nil {
		t.Fatalf("SharesListFiles failed: %v", err)
	}
	if len(shareFiles.Items) != 1 {
		t.Fatalf("expected one shared file, got items=%d", len(shareFiles.Items))
	}

	if err := client.FilesCreateShare(ctx, &api.FileShareCreate{Password: api.NewOptString("pw1")}, api.FilesCreateShareParams{ID: folder.ID.Value}); err != nil {
		t.Fatalf("FilesCreateShare protected failed: %v", err)
	}

	shares2, err := client.FilesListShares(ctx, api.FilesListSharesParams{ID: folder.ID.Value})
	if err != nil {
		t.Fatalf("FilesListShares protected failed: %v", err)
	}

	protectedShareID := ""
	for _, sh := range shares2 {
		if sh.Protected {
			protectedShareID = sh.ID
			break
		}
	}
	if protectedShareID == "" {
		t.Fatalf("expected protected share")
	}

	_, err = public.SharesListFiles(ctx, api.SharesListFilesParams{ID: protectedShareID, Limit: api.NewOptInt(20), Sort: api.NewOptShareQuerySort(api.ShareQuerySortName), Order: api.NewOptShareQueryOrder(api.ShareQueryOrderAsc)})
	if statusCode(err) != 401 {
		t.Fatalf("expected 401 before unlock, got %d err=%v", statusCode(err), err)
	}

	_, err = public.SharesUnlock(ctx, &api.ShareUnlock{Password: "wrong"}, api.SharesUnlockParams{ID: protectedShareID})
	if statusCode(err) != 403 {
		t.Fatalf("expected 403 for wrong password, got %d err=%v", statusCode(err), err)
	}
	unlockRes, err := public.SharesUnlock(ctx, &api.ShareUnlock{Password: "pw1"}, api.SharesUnlockParams{ID: protectedShareID})
	if err != nil {
		t.Fatalf("SharesUnlock correct password failed: %v", err)
	}
	if unlockRes.SetCookie == "" {
		t.Fatalf("expected share unlock cookie")
	}
	_, err = public.SharesListFiles(ctx, api.SharesListFilesParams{ID: protectedShareID, Limit: api.NewOptInt(20), Sort: api.NewOptShareQuerySort(api.ShareQuerySortName), Order: api.NewOptShareQueryOrder(api.ShareQueryOrderAsc)})
	if err != nil {
		t.Fatalf("expected protected share access after unlock, got %v", err)
	}
}

func TestSharesRoutes_Validation(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	public, _, _ := loginWithClient(t, s, 7205, "user7205")

	_, err := public.SharesGetById(ctx, api.SharesGetByIdParams{ID: "bad-uuid"})
	if statusCode(err) != 400 {
		t.Fatalf("expected 400, got %d err=%v", statusCode(err), err)
	}

	_, err = public.SharesUnlock(ctx, &api.ShareUnlock{Password: "x"}, api.SharesUnlockParams{ID: "bad-uuid"})
	if statusCode(err) != 400 {
		t.Fatalf("expected 400, got %d err=%v", statusCode(err), err)
	}
}
