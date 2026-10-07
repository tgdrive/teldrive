//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	api "github.com/tgdrive/teldrive/v2/internal/api"
	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/health"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

func TestFileListingReportsWhatEachFolderHolds(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (2001), (2002)"); err != nil {
		t.Fatal(err)
	}
	media, sub, deep, empty, trashedFolder, other := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	topFile := uuid.New()
	// Media holds 100 + (Sub: 20 + (Deep: 3)) = 123. A trashed file under it is not counted, an
	// empty folder is 0, and another user's bytes are theirs.
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,parent_id,name,kind,mime_type,size,status,mod_time,updated_at,deleted_at) VALUES
($1,2001,NULL,'Media','folder',NULL,NULL,'active',now(),now(),NULL),
($2,2001,$1,'Sub','folder',NULL,NULL,'active',now(),now(),NULL),
($3,2001,$2,'Deep','folder',NULL,NULL,'active',now(),now(),NULL),
($4,2001,NULL,'Empty','folder',NULL,NULL,'active',now(),now(),NULL),
($5,2001,NULL,'OldStuff','folder',NULL,NULL,'trashed',now(),now(),now()),
($6,2002,NULL,'Other','folder',NULL,NULL,'active',now(),now(),NULL),
($7,2001,NULL,'top.bin','file','application/octet-stream',7,'active',now(),now(),NULL),
(gen_random_uuid(),2001,$1,'a.bin','file','application/octet-stream',100,'active',now(),now(),NULL),
(gen_random_uuid(),2001,$2,'b.bin','file','application/octet-stream',20,'active',now(),now(),NULL),
(gen_random_uuid(),2001,$3,'c.bin','file','application/octet-stream',3,'active',now(),now(),NULL),
(gen_random_uuid(),2001,$2,'gone.bin','file','application/octet-stream',1000,'trashed',now(),now(),now()),
(gen_random_uuid(),2001,$5,'old.bin','file','application/octet-stream',50,'active',now(),now(),NULL),
(gen_random_uuid(),2002,$6,'theirs.bin','file','application/octet-stream',5000,'active',now(),now(),NULL)
`, media, sub, deep, empty, trashedFolder, other, topFile); err != nil {
		t.Fatal(err)
	}

	catalogService := catalog.NewService(db.Pool, nil)

	// The catalog: a folder with nothing is 0; what is not one of the user's folders gets no entry
	// (another user's folder, a file, an id that is nothing).
	sizes, err := catalogService.FolderSizes(ctx, 2001, []uuid.UUID{media, sub, deep, empty, other, topFile, uuid.New()}, 0)
	if err != nil {
		t.Fatalf("FolderSizes() error = %v", err)
	}
	want := map[uuid.UUID]int64{media: 123, sub: 23, deep: 3, empty: 0}
	if len(sizes) != len(want) {
		t.Fatalf("FolderSizes() = %v, want %v", sizes, want)
	}
	for id, size := range want {
		if got, ok := sizes[id]; !ok || got != size {
			t.Fatalf("FolderSizes()[%s] = %d, %v; want %d", id, got, ok, size)
		}
	}
	if none, err := catalogService.FolderSizes(ctx, 2001, nil, 0); err != nil || len(none) != 0 {
		t.Fatalf("FolderSizes(no ids) = %v, %v", none, err)
	}
	// Within a budget it is the same answer.
	if within, err := catalogService.FolderSizes(ctx, 2001, []uuid.UUID{media}, catalog.FolderSizeBudget); err != nil || within[media] != 123 {
		t.Fatalf("FolderSizes(within the budget) = %v, %v", within, err)
	}

	handler := api.NewHandler(catalogService, uploads.NewService(db.Pool), nil, nil, health.NewService("test", db.Pool), 0, nil)
	server, err := api.NewServer(handler, api.NewSecurity(listingAuthenticator{}))
	if err != nil {
		t.Fatal(err)
	}
	type entry struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
		Size *int64 `json:"size"`
	}
	type listing struct {
		Items      []entry `json:"items"`
		NextCursor string  `json:"nextCursor"`
	}
	list := func(token string, params url.Values) listing {
		t.Helper()
		var response *httptest.ResponseRecorder = performRequest(t, server, http.MethodGet, "/v1/files?"+params.Encode(), nil, map[string]string{"Authorization": "Bearer " + token})
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
		}
		var got listing
		if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode listing: %v; body=%s", err, response.Body.String())
		}
		return got
	}
	sizeOf := func(got listing, name string) *int64 {
		t.Helper()
		for _, item := range got.Items {
			if item.Name == name {
				return item.Size
			}
		}
		t.Fatalf("%s is not in the listing: %+v", name, got.Items)
		return nil
	}
	expect := func(got listing, name string, size int64) {
		t.Helper()
		if value := sizeOf(got, name); value == nil || *value != size {
			t.Fatalf("%s size = %v, want %d", name, value, size)
		}
	}

	// The root: each folder with what is under it at any depth, a file with its own size.
	root := list("user-2001", url.Values{})
	expect(root, "Media", 123)
	expect(root, "Empty", 0)
	expect(root, "top.bin", 7)

	// Inside a folder, the same of its folders.
	inside := list("user-2001", url.Values{"parentId": {media.String()}})
	expect(inside, "Sub", 23)
	expect(inside, "a.bin", 100)

	// The other user sees their own bytes, and nothing of these.
	theirs := list("user-2002", url.Values{})
	expect(theirs, "Other", 5000)
	if len(theirs.Items) != 1 {
		t.Fatalf("the other user's root = %+v", theirs.Items)
	}

	// A drive-wide search is of active files too, and its folders carry a size.
	found := list("user-2001", url.Values{"scope": {"drive"}, "search": {"Sub"}})
	expect(found, "Sub", 23)

	// The trash is listed as it was: a trashed folder has no size.
	trash := list("user-2001", url.Values{"status": {"trashed"}})
	if value := sizeOf(trash, "OldStuff"); value != nil {
		t.Fatalf("a trashed folder has size %d", *value)
	}

	// A folder holding more than can be summed within the budget: the sum is given up, by name, and
	// the connection it was on is as good as before.
	big := uuid.New()
	if _, err := db.Pool.Exec(ctx, `INSERT INTO files (id,user_id,parent_id,name,kind,status,mod_time) VALUES ($1,2001,NULL,'Big','folder','active',now())`, big); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,parent_id,name,kind,mime_type,size,status,mod_time)
SELECT gen_random_uuid(),2001,$1,'f'||g||'.bin','file','application/octet-stream',1,'active',now() FROM generate_series(1,20000) g`, big); err != nil {
		t.Fatal(err)
	}
	if _, err := catalogService.FolderSizes(ctx, 2001, []uuid.UUID{big}, time.Millisecond); !errors.Is(err, catalog.ErrFolderSizesOverBudget) {
		t.Fatalf("FolderSizes(over the budget) error = %v", err)
	}
	if all, err := catalogService.FolderSizes(ctx, 2001, []uuid.UUID{big, media}, 0); err != nil || all[big] != 20000 || all[media] != 123 {
		t.Fatalf("FolderSizes(after giving one up) = %v, %v", all, err)
	}
	if _, err := db.Pool.Exec(ctx, `DELETE FROM files WHERE id = $1 OR parent_id = $1`, big); err != nil {
		t.Fatal(err)
	}

	// Sorted by size and read a page at a time, every entry comes once: the cursor is made from the
	// stored size (none, for a folder), not from what the folder holds.
	for _, order := range []string{"asc", "desc"} {
		var names []string
		cursor := ""
		for page := 0; page < 10; page++ {
			params := url.Values{"sort": {"size"}, "order": {order}, "limit": {"1"}}
			if cursor != "" {
				params.Set("cursor", cursor)
			}
			got := list("user-2001", params)
			for _, item := range got.Items {
				names = append(names, item.Name)
				if item.Name == "Media" && (item.Size == nil || *item.Size != 123) {
					t.Fatalf("Media on a size-sorted page has size %v", item.Size)
				}
			}
			if cursor = got.NextCursor; cursor == "" {
				break
			}
		}
		slices.Sort(names)
		if !slices.Equal(names, []string{"Empty", "Media", "top.bin"}) {
			t.Fatalf("paged by size %s = %v", order, names)
		}
	}
}
