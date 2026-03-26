package integration_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tgdrive/teldrive/internal/api"
)

func TestFilesRoutes_CRUDAndOperations(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	_, client, _ := loginWithClient(t, s, 7202, "user7202")

	created, err := client.FilesCreate(ctx, &api.File{Name: "docs", Type: api.FileTypeFolder, Path: api.NewOptString("/")})
	if err != nil {
		t.Fatalf("FilesCreate folder failed: %v", err)
	}
	if !created.ID.IsSet() || uuid.UUID(created.ID.Value) == uuid.Nil {
		t.Fatalf("created folder id is empty")
	}

	createdGet, err := client.FilesGetById(ctx, api.FilesGetByIdParams{ID: created.ID.Value})
	if err != nil {
		t.Fatalf("FilesGetById folder failed: %v", err)
	}
	if createdGet.Name != "docs" || createdGet.Type != api.FileTypeFolder {
		t.Fatalf("unexpected folder payload: %+v", createdGet)
	}

	listAll, err := client.FilesList(ctx, api.FilesListParams{Limit: api.NewOptInt(100)})
	if err != nil {
		t.Fatalf("FilesList failed: %v", err)
	}
	if len(listAll.Items) < 1 {
		t.Fatalf("expected file list items >= 1, got %d", len(listAll.Items))
	}

	children, err := client.FilesChildren(ctx, api.FilesChildrenParams{ID: created.ID.Value, Limit: api.NewOptInt(50)})
	if err != nil {
		t.Fatalf("FilesChildren failed: %v", err)
	}
	if len(children.Items) != 0 {
		t.Fatalf("expected empty children for new folder, got %d", len(children.Items))
	}

	dataFile, err := client.FilesCreate(ctx, &api.File{
		Name:      "report.txt",
		Type:      api.FileTypeFile,
		Path:      api.NewOptString("/"),
		MimeType:  api.NewOptString("text/plain"),
		ChannelId: api.NewOptInt64(900001),
		Size:      api.NewOptInt64(42),
		Encrypted: api.NewOptBool(true),
	})
	if err != nil {
		t.Fatalf("FilesCreate file failed: %v", err)
	}

	updated, err := client.FilesUpdate(ctx, &api.FileUpdate{Name: api.NewOptString("docs-updated")}, api.FilesUpdateParams{ID: created.ID.Value})
	if err != nil {
		t.Fatalf("FilesUpdate folder failed: %v", err)
	}
	if updated.Name != "docs-updated" {
		t.Fatalf("expected updated folder name docs-updated, got %s", updated.Name)
	}

	updatedFile, err := client.FilesUpdate(ctx, &api.FileUpdate{
		Name:      api.NewOptString("report-updated.txt"),
		ChannelId: api.NewOptInt64(900002),
		Encrypted: api.NewOptBool(false),
		UpdatedAt: api.NewOptDateTime(createdGet.UpdatedAt.Value.Add(5 * time.Minute)),
	}, api.FilesUpdateParams{ID: dataFile.ID.Value})
	if err != nil {
		t.Fatalf("FilesUpdate file failed: %v", err)
	}
	if updatedFile.Name != "report-updated.txt" {
		t.Fatalf("expected updated file name, got %s", updatedFile.Name)
	}
	wantUpdatedAt := createdGet.UpdatedAt.Value.Add(5 * time.Minute)
	if !updatedFile.UpdatedAt.IsSet() || !updatedFile.UpdatedAt.Value.Equal(wantUpdatedAt) {
		t.Fatalf("expected updatedAt=%s, got %+v", wantUpdatedAt.UTC().Format(time.RFC3339Nano), updatedFile.UpdatedAt)
	}

	archive, err := client.FilesCreate(ctx, &api.File{Name: "archive", Type: api.FileTypeFolder, Path: api.NewOptString("/")})
	if err != nil {
		t.Fatalf("FilesCreate archive failed: %v", err)
	}

	updatedParent, err := client.FilesUpdate(ctx, &api.FileUpdate{ParentId: api.NewOptUUID(archive.ID.Value)}, api.FilesUpdateParams{ID: created.ID.Value})
	if err != nil {
		t.Fatalf("FilesUpdate parent failed: %v", err)
	}
	if !updatedParent.ParentId.IsSet() || updatedParent.ParentId.Value != archive.ID.Value {
		t.Fatalf("expected parentId=%s, got %+v", archive.ID.Value, updatedParent.ParentId)
	}

	if err := client.FilesDeleteById(ctx, api.FilesDeleteByIdParams{ID: dataFile.ID.Value}); err != nil {
		t.Fatalf("FilesDeleteById file failed: %v", err)
	}
	trashedList, err := client.FilesList(ctx, api.FilesListParams{Status: api.NewOptFileQueryStatus(api.FileQueryStatusTrashed), Limit: api.NewOptInt(50)})
	if err != nil {
		t.Fatalf("FilesList trashed failed: %v", err)
	}
	trashedFound := false
	for _, item := range trashedList.Items {
		if item.ID.IsSet() && item.ID.Value == dataFile.ID.Value {
			trashedFound = true
			break
		}
	}
	if !trashedFound {
		t.Fatalf("expected deleted file in trashed listing")
	}
	if err := client.FilesRestore(ctx, api.FilesRestoreParams{ID: dataFile.ID.Value}); err != nil {
		t.Fatalf("FilesRestore file failed: %v", err)
	}

	purgeFile, err := client.FilesCreate(ctx, &api.File{
		Name:      "purge.txt",
		Type:      api.FileTypeFile,
		Path:      api.NewOptString("/"),
		MimeType:  api.NewOptString("text/plain"),
		ChannelId: api.NewOptInt64(900003),
		Size:      api.NewOptInt64(7),
	})
	if err != nil {
		t.Fatalf("FilesCreate purge file failed: %v", err)
	}
	if err := client.FilesDelete(ctx, &api.FileDelete{Ids: []api.UUID{purgeFile.ID.Value}}, api.FilesDeleteParams{}); err != nil {
		t.Fatalf("FilesDelete trash failed: %v", err)
	}
	if err := client.FilesDelete(ctx, &api.FileDelete{Ids: []api.UUID{purgeFile.ID.Value}}, api.FilesDeleteParams{Force: api.NewOptBool(true)}); err != nil {
		t.Fatalf("FilesDelete force failed: %v", err)
	}

}

func TestFilesRoutes_Validation(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	_, client, _ := loginWithClient(t, s, 7203, "user7203")

	t.Run("FilesGetById invalid UUID => 400", func(t *testing.T) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.server.URL+"/files/bad-uuid", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		resp, err := s.httpCli.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("FilesCreate missing path/parent => 409", func(t *testing.T) {
		_, err := client.FilesCreate(ctx, &api.File{Name: "docs", Type: api.FileTypeFolder})
		if statusCode(err) != 409 {
			t.Fatalf("expected 409, got %d err=%v", statusCode(err), err)
		}
	})

	t.Run("JobsInsert unknown kind => 400", func(t *testing.T) {
		_, err := client.JobsInsert(ctx, &api.JobInsertRequest{Kind: "unknown.kind", Args: []byte("{}")})
		if statusCode(err) != 400 {
			t.Fatalf("expected 400, got %d err=%v", statusCode(err), err)
		}
	})

}
