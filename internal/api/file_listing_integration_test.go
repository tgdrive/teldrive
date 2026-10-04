//go:build integration

package api_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	api "github.com/tgdrive/teldrive/v2/internal/api"
	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/health"
	"github.com/tgdrive/teldrive/v2/internal/shares"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

func TestGeneratedServerFileListingScopesAndGrantIsolation(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (2001), (2002)"); err != nil {
		t.Fatal(err)
	}
	ownerRoot, shared, foreignLeaf := uuid.New(), uuid.New(), uuid.New()
	userRoot, userChild, userSecond, userNestedFolder := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,parent_id,name,kind,mime_type,size,status,mod_time,updated_at,deleted_at) VALUES
($1,2001,NULL,'OwnerRoot','folder',NULL,NULL,'active',now(),now(),NULL),
($2,2001,$1,'SharedNested','folder',NULL,NULL,'active',now(),now(),NULL),
($3,2001,$2,'foreign-leaf.txt','file','text/plain',1,'active',now(),now(),NULL),
($4,2002,NULL,'UserRoot','folder',NULL,NULL,'active',now(),now(),NULL),
($5,2002,$4,'alpha.txt','file','text/plain',1,'active',now() - interval '2 minutes',now(),NULL),
($6,2002,$4,'beta.txt','file','text/plain',1,'active',now() - interval '1 minute',now(),NULL),
($7,2002,$4,'Nested','folder',NULL,NULL,'active',now(),now(),NULL)
`, ownerRoot, shared, foreignLeaf, userRoot, userChild, userSecond, userNestedFolder); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO file_access_grants (file_id,owner_id,grantee_id,permission) VALUES ($1,2001,2002,'read')`, shared); err != nil {
		t.Fatal(err)
	}
	shareService, err := shares.NewService(db.Pool, catalog.NewService(db.Pool, nil))
	if err != nil {
		t.Fatal(err)
	}
	handler := api.NewHandler(catalog.NewService(db.Pool, nil), uploads.NewService(db.Pool), nil, nil, health.NewService("test", db.Pool), 0, nil).
		ConfigureDomains(nil, nil, nil, nil, shareService, nil)
	server, err := api.NewServer(handler, api.NewSecurity(listingAuthenticator{}))
	if err != nil {
		t.Fatal(err)
	}
	request := func(token string, params url.Values) *httptest.ResponseRecorder {
		t.Helper()
		return performRequest(t, server, http.MethodGet, "/v1/files?"+params.Encode(), nil, map[string]string{"Authorization": "Bearer " + token})
	}
	assertStatus := func(response *httptest.ResponseRecorder, expected int) {
		t.Helper()
		if response.Code != expected {
			t.Fatalf("status = %d, want %d; body=%s", response.Code, expected, response.Body.String())
		}
	}
	type listing struct {
		Items      []map[string]json.RawMessage `json:"items"`
		NextCursor string                       `json:"nextCursor"`
	}
	decode := func(response *httptest.ResponseRecorder) listing {
		t.Helper()
		var got listing
		if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode listing: %v; body=%s", err, response.Body.String())
		}
		return got
	}
	fileNames := func(got listing) []string {
		t.Helper()
		var names []string
		for _, item := range got.Items {
			var name string
			if err := json.Unmarshal(item["name"], &name); err != nil {
				t.Fatalf("decode item name: %v", err)
			}
			names = append(names, name)
		}
		return names
	}

	// A real nested grant continues to authorize legacy folder browsing, without
	// changing the legacy response shape to expose a parent path.
	legacy := request("user-2002", url.Values{"parentId": {shared.String()}})
	assertStatus(legacy, http.StatusOK)
	legacyResult := decode(legacy)
	if len(legacyResult.Items) != 1 || string(legacyResult.Items[0]["name"]) != `"foreign-leaf.txt"` {
		t.Fatalf("legacy shared-folder items = %#v", fileNames(legacyResult))
	}
	if _, exists := legacyResult.Items[0]["parentPath"]; exists {
		t.Fatal("legacy shared-folder response unexpectedly contains parentPath")
	}

	driveParams := url.Values{"scope": {"drive"}, "limit": {"1"}}
	drive := request("user-2002", driveParams)
	assertStatus(drive, http.StatusOK)
	driveResult := decode(drive)
	if names := fileNames(driveResult); len(names) != 1 || names[0] != "alpha.txt" {
		t.Fatalf("drive first page = %v; granted foreign files must be excluded", names)
	}
	if path := string(driveResult.Items[0]["parentPath"]); path != `"/UserRoot"` {
		t.Fatalf("drive nested parentPath = %s, want /UserRoot", path)
	}
	if driveResult.NextCursor == "" {
		t.Fatal("drive first page omitted nextCursor")
	}
	driveParams.Set("cursor", driveResult.NextCursor)
	drivePage2 := request("user-2002", driveParams)
	assertStatus(drivePage2, http.StatusOK)
	page2 := decode(drivePage2)
	if names := fileNames(page2); len(names) != 1 || names[0] != "beta.txt" {
		t.Fatalf("drive second page = %v", names)
	}
	rootPath := request("user-2002", url.Values{"scope": {"drive"}, "search": {"UserRoot"}})
	assertStatus(rootPath, http.StatusOK)
	if path := string(decode(rootPath).Items[0]["parentPath"]); path != `"/"` {
		t.Fatalf("drive root parentPath = %s, want /", path)
	}

	recursiveParams := url.Values{"scope": {"recursive"}, "parentId": {userRoot.String()}, "limit": {"1"}}
	recursive := request("user-2002", recursiveParams)
	assertStatus(recursive, http.StatusOK)
	recursiveResult := decode(recursive)
	if names := fileNames(recursiveResult); len(names) != 1 || names[0] != "alpha.txt" {
		t.Fatalf("recursive listing = %v", names)
	}
	if path := string(recursiveResult.Items[0]["parentPath"]); path != `"/UserRoot"` {
		t.Fatalf("recursive parentPath = %s, want /UserRoot", path)
	}
	if recursiveResult.NextCursor == "" {
		t.Fatal("recursive first page omitted nextCursor")
	}
	recursiveParams.Set("cursor", recursiveResult.NextCursor)
	recursivePage2 := request("user-2002", recursiveParams)
	assertStatus(recursivePage2, http.StatusOK)
	if names := fileNames(decode(recursivePage2)); len(names) != 1 || names[0] != "beta.txt" {
		t.Fatalf("recursive second page = %v", names)
	}
	foreignRecursive := request("user-2002", url.Values{"scope": {"recursive"}, "parentId": {shared.String()}})
	assertStatus(foreignRecursive, http.StatusUnprocessableEntity)
	if got := foreignRecursive.Body.String(); contains(got, "OwnerRoot") || contains(got, "SharedNested") {
		t.Fatalf("foreign recursive error disclosed ancestor path: %s", got)
	}

	// Cursors are bound to every meaningful advanced-listing dimension.
	filtered := url.Values{
		"scope": {"drive"}, "limit": {"1"}, "search": {"alpha"}, "kind": {"file"},
		"category": {"document"}, "updatedAfter": {time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)},
		"sort": {"name"}, "order": {"asc"},
	}
	filteredFirst := request("user-2002", filtered)
	assertStatus(filteredFirst, http.StatusOK)
	filteredResult := decode(filteredFirst)
	if filteredResult.NextCursor == "" {
		t.Fatal("filtered drive page omitted nextCursor")
	}
	mutations := []struct {
		name   string
		change func(url.Values)
	}{
		{"scope", func(v url.Values) { v.Set("scope", "recursive"); v.Set("parentId", userRoot.String()) }},
		{"text", func(v url.Values) { v.Set("search", "beta") }},
		{"kind", func(v url.Values) { v.Set("kind", "folder") }},
		{"category", func(v url.Values) { v.Set("category", "image") }},
		{"date", func(v url.Values) { v.Set("updatedAfter", time.Now().UTC().Format(time.RFC3339)) }},
		{"sort", func(v url.Values) { v.Set("sort", "updatedAt") }},
	}
	for _, mutation := range mutations {
		t.Run("cursor rejects "+mutation.name, func(t *testing.T) {
			params, err := url.ParseQuery(filtered.Encode())
			if err != nil {
				t.Fatal(err)
			}
			params.Set("cursor", filteredResult.NextCursor)
			mutation.change(params)
			assertStatus(request("user-2002", params), http.StatusUnprocessableEntity)
		})
	}
	recursiveCursorParams := url.Values{"scope": {"recursive"}, "parentId": {userRoot.String()}, "limit": {"1"}, "cursor": {recursiveResult.NextCursor}}
	recursiveFolderMutation := url.Values{}
	for key, values := range recursiveCursorParams {
		recursiveFolderMutation[key] = append([]string(nil), values...)
	}
	recursiveFolderMutation.Set("parentId", userNestedFolder.String())
	assertStatus(request("user-2002", recursiveFolderMutation), http.StatusUnprocessableEntity)

	legacyPaged := url.Values{"parentId": {userRoot.String()}, "limit": {"1"}}
	legacyFirst := request("user-2002", legacyPaged)
	assertStatus(legacyFirst, http.StatusOK)
	legacyPage := decode(legacyFirst)
	if legacyPage.NextCursor == "" {
		t.Fatal("legacy folder page omitted nextCursor")
	}
	encodedCursor, err := base64.RawURLEncoding.DecodeString(legacyPage.NextCursor)
	if err != nil {
		t.Fatalf("decode emitted folder cursor: %v", err)
	}
	var legacyCursor map[string]json.RawMessage
	if err := json.Unmarshal(encodedCursor, &legacyCursor); err != nil {
		t.Fatalf("unmarshal emitted folder cursor: %v", err)
	}
	delete(legacyCursor, "scope")
	delete(legacyCursor, "folder_id")
	delete(legacyCursor, "fingerprint")
	legacyCursorJSON, err := json.Marshal(legacyCursor)
	if err != nil {
		t.Fatalf("marshal legacy folder cursor: %v", err)
	}
	oldFolderCursor := base64.RawURLEncoding.EncodeToString(legacyCursorJSON)
	legacyPaged.Set("cursor", oldFolderCursor)
	legacyContinuation := request("user-2002", legacyPaged)
	assertStatus(legacyContinuation, http.StatusOK)
	if names := fileNames(decode(legacyContinuation)); len(names) != 1 || names[0] != "beta.txt" {
		t.Fatalf("old-format folder cursor continuation = %v", names)
	}
	legacyPaged.Del("parentId")
	legacyPaged.Set("scope", "drive")
	assertStatus(request("user-2002", legacyPaged), http.StatusUnprocessableEntity)
	legacyPaged.Set("scope", "recursive")
	legacyPaged.Set("parentId", userRoot.String())
	assertStatus(request("user-2002", legacyPaged), http.StatusUnprocessableEntity)

	for _, invalid := range []url.Values{
		{"scope": {"drive"}, "path": {"UserRoot"}},
		{"scope": {"recursive"}, "parentId": {userRoot.String()}, "path": {"UserRoot"}},
		{"scope": {"drive"}, "parentId": {userRoot.String()}},
		{"scope": {"drive"}, "status": {"trashed"}},
		{"scope": {"recursive"}, "parentId": {userRoot.String()}, "status": {"trashed"}},
	} {
		assertStatus(request("user-2002", invalid), http.StatusUnprocessableEntity)
	}
}

type listingAuthenticator struct{}

func (listingAuthenticator) AuthenticateBearer(_ context.Context, token string) (api.Identity, error) {
	switch token {
	case "user-2001":
		return api.Identity{UserID: 2001, Roles: []string{"user"}}, nil
	case "user-2002":
		return api.Identity{UserID: 2002, Roles: []string{"user"}}, nil
	default:
		return api.Identity{}, api.ErrUnauthenticated
	}
}
func (a listingAuthenticator) AuthenticateAPIKey(ctx context.Context, key string) (api.Identity, error) {
	return a.AuthenticateBearer(ctx, key)
}

func contains(value, substr string) bool { return strings.Contains(value, substr) }
