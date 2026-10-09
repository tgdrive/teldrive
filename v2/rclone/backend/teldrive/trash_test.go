package teldrive

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rclone/rclone/backend/teldrive/api"
	"github.com/rclone/rclone/fs/config/configmap"
)

func TestTrashListingDoesNotDuplicateGlobalRoots(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(apiKeyHeaderName) != "test-key" {
			t.Error("API key missing")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/me" {
			_ = json.NewEncoder(w).Encode(api.UserProfile{UserId: 1001})
			return
		}
		if r.URL.Path != "/api/v1/files" {
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		folderID := "6fe3a4b5-c965-49c6-9131-8509b335212d"
		page := api.ReadMetadataResponse{Files: []api.FileInfo{}}
		if r.URL.Query().Get("status") == "trashed" {
			page.Files = []api.FileInfo{{Id: "deleted", Name: "audio.mp3", ParentId: folderID, Kind: "file", Status: "trashed"}}
		} else if r.URL.Query().Get("parentId") == "" {
			page.Files = []api.FileInfo{{Id: folderID, Name: "music", Kind: "folder", Status: "active"}}
		}
		_ = json.NewEncoder(w).Encode(page)
	}))
	defer server.Close()
	f, err := NewFs(context.Background(), "test", "", configmap.Simple{"api_host": server.URL, "api_key": "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := f.(*Fs).listTopLevelTrash(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "audio.mp3" {
		t.Fatalf("trash entries = %+v", entries)
	}
}
