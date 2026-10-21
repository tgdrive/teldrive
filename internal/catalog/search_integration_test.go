//go:build integration

package catalog_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/catalog"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

type searchQueryProbe struct {
	sql  string
	args []any
}

func (p *searchQueryProbe) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.HasPrefix(data.SQL, "-- name: ListFiles") {
		p.sql = data.SQL
		p.args = slices.Clone(data.Args)
	}
	return ctx
}

func (*searchQueryProbe) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestFilenameSearchWordsAndIndex(t *testing.T) {
	db := testpostgres.New(t)
	ctx := context.Background()
	seedUser(t, db.Pool, 1001)
	seedUser(t, db.Pool, 2002)
	_, err := db.Pool.Exec(ctx, `
INSERT INTO files (id, user_id, name, kind, size, encryption, status, mod_time, deleted_at)
VALUES
    (gen_random_uuid(), 1001, 'The.Terminal.List.S02.720p.10bit.WEBRip.2CH.x265.HEVC-PSA', 'file', 1, false, 'active', now(), NULL),
    (gen_random_uuid(), 1001, 'Terminal - complete collection - THE.mkv', 'file', 1, false, 'active', now(), NULL),
    (gen_random_uuid(), 1001, 'The.Other.Series.S02.720p.mkv', 'file', 1, false, 'active', now(), NULL),
    (gen_random_uuid(), 1001, 'Terminal.documentary.720p.mkv', 'file', 1, false, 'active', now(), NULL),
    (gen_random_uuid(), 1001, 'The.Terminal.deleted.mkv', 'file', 1, false, 'trashed', now(), now()),
    (gen_random_uuid(), 2002, 'The.Terminal.private.mkv', 'file', 1, false, 'active', now(), NULL);
INSERT INTO files (id, user_id, name, kind, size, encryption, status, mod_time)
SELECT gen_random_uuid(), 1001, md5(i::text) || '.unrelated.mkv', 'file', 1, false, 'active', now()
FROM generate_series(1, 3000) i;`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, "VACUUM ANALYZE files"); err != nil {
		t.Fatal(err)
	}

	probe := &searchQueryProbe{}
	config := db.Pool.Config()
	config.ConnConfig.Tracer = probe
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	svc := catalog.NewService(pool, nil)
	for _, scope := range []string{"folder", "drive"} {
		t.Run(scope, func(t *testing.T) {
			for _, tc := range []struct {
				search, searchType string
				want               int
			}{
				{"The Terminal", "text", 2},
				{"Terminal The", "text", 2},
				{"  THE   terminal  ", "text", 2},
				{"the\tterminal", "text", 2},
				{"the\nterminal", "text", 2},
				{"the termi", "text", 2},
				{"Terminal 720p", "text", 2},
				{"The Terminal 720p", "text", 1},
				{"The Terminal nonexistent", "text", 0},
				{"The Terminal", "regex", 0},
				{`The\.Terminal`, "regex", 1},
			} {
				t.Run(tc.search+"/"+tc.searchType, func(t *testing.T) {
					items, err := svc.List(ctx, catalog.ListInput{UserID: 1001, Scope: scope, Search: tc.search, SearchType: tc.searchType})
					if err != nil {
						t.Fatal(err)
					}
					if len(items) != tc.want {
						var names []string
						for _, item := range items {
							names = append(names, item.Name)
						}
						t.Fatalf("got %q, want %d matches", names, tc.want)
					}
					if tc.search != "The Terminal" || tc.searchType != "text" {
						return
					}
					rows, err := db.Pool.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+probe.sql, probe.args...)
					if err != nil {
						t.Fatal(err)
					}
					defer rows.Close()
					var plan strings.Builder
					for rows.Next() {
						var line string
						if err := rows.Scan(&line); err != nil {
							t.Fatal(err)
						}
						plan.WriteString(line + "\n")
					}
					if err := rows.Err(); err != nil {
						t.Fatal(err)
					}
					t.Log(plan.String())
					if !strings.Contains(plan.String(), "Bitmap Index Scan on files_name_search_idx") {
						t.Fatal("search did not use the filename GIN index")
					}
				})
			}
		})
	}
}
