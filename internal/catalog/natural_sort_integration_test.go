//go:build integration

package catalog_test

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestNaturalNameSortingAndPagination(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	seedUser(t, db.Pool, 1001)
	svc := catalog.NewService(db.Pool, nil)

	// Include multiple digit runs, leading zeros, Unicode, and numbers too large
	// for a bigint. Alternate files and folders: both use the same name order.
	want := []string{
		"entry1.txt", "entry02.txt", "entry2.txt", "entry3.txt",
		"entry10.txt", "entry11.txt", "entry20.txt",
		"entry100000000000000000000000000000.txt",
		"entry200000000000000000000000000000.txt",
		"season2episode2.txt", "season2episode10.txt", "season10episode1.txt",
		"日本語2.txt", "日本語10.txt",
	}
	// Insert in reverse order so insertion order cannot satisfy the assertions.
	for i := len(want) - 1; i >= 0; i-- {
		kind := "file"
		var size *int64
		if i%2 == 0 {
			kind = "folder"
		} else {
			value := int64(1)
			size = &value
		}
		if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,name,kind,size,status,mod_time)
VALUES ($1,1001,$2,$3,$4,'active',now())`, uuid.New(), want[i], kind, size); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		name, order, searchType string
	}{
		{"basic", "asc", "text"},
		{"advanced_ascending", "asc", "regex"},
		{"advanced_descending", "desc", "regex"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expected := slices.Clone(want)
			if tc.order == "desc" {
				slices.Reverse(expected)
			}
			for _, pageSize := range []int32{1, 3, 100} {
				input := catalog.ListInput{UserID: 1001, Sort: "name", Order: tc.order, SearchType: tc.searchType, Limit: pageSize}
				var got []string
				for {
					page, err := svc.List(ctx, input)
					if err != nil {
						t.Fatal(err)
					}
					if len(page) == 0 {
						break
					}
					for _, file := range page {
						got = append(got, file.Name)
					}
					if len(got) > len(expected) {
						t.Fatalf("pagination repeated entries: %v", got)
					}
					last := page[len(page)-1]
					id := uuid.UUID(last.ID.Bytes)
					input.AfterID, input.AfterName = &id, last.Name
					if tc.searchType == "regex" {
						input.AfterValue = catalog.FileCursorValue(last, "name")
					}
				}
				if !slices.Equal(got, expected) {
					t.Fatalf("page size %d: names = %v, want %v", pageSize, got, expected)
				}
			}
		})
	}
}

func TestNaturalNameSortingDuplicateNamesAcrossFolders(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	seedUser(t, db.Pool, 1001)
	svc := catalog.NewService(db.Pool, nil)
	folderIDs := []uuid.UUID{uuid.New(), uuid.New()}
	for _, id := range folderIDs {
		if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,name,kind,status,mod_time)
VALUES ($1,1001,$2,'folder','active',now())`, id, id.String()); err != nil {
			t.Fatal(err)
		}
	}
	ids := []uuid.UUID{
		uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		uuid.MustParse("00000000-0000-0000-0000-000000000003"),
	}
	for i, name := range []string{"file2.txt", "file2.txt", "file10.txt"} {
		if _, err := db.Pool.Exec(ctx, `
INSERT INTO files (id,user_id,parent_id,name,kind,size,status,mod_time)
VALUES ($1,1001,$2,$3,'file',1,'active',now())`, ids[i], folderIDs[i%2], name); err != nil {
			t.Fatal(err)
		}
	}
	for _, order := range []string{"asc", "desc"} {
		t.Run(order, func(t *testing.T) {
			expected := slices.Clone(ids)
			if order == "desc" {
				slices.Reverse(expected)
			}
			kind := sqlcgen.FileKindFile
			input := catalog.ListInput{UserID: 1001, Scope: "drive", Kind: &kind, Sort: "name", Order: order, Limit: 1}
			for _, wantID := range expected {
				page, err := svc.List(ctx, input)
				if err != nil {
					t.Fatal(err)
				}
				if len(page) != 1 || page[0].ID != dbtypes.UUID(wantID) {
					t.Fatalf("page = %#v, want ID %s", page, wantID)
				}
				input.AfterID = &wantID
				input.AfterValue = catalog.FileCursorValue(page[0], "name")
			}
			if page, err := svc.List(ctx, input); err != nil || len(page) != 0 {
				t.Fatalf("final page = %#v, %v; want empty", page, err)
			}
		})
	}
}
