package performance

import (
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tgdrive/teldrive/internal/database"
	"github.com/tgdrive/teldrive/pkg/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var perfDB *gorm.DB

func TestMain(m *testing.M) {
	dsn := os.Getenv("TELDRIVE_DB_DATASOURCE")

	var err error
	perfDB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}

	// Ensure migrations (using internal package logic if possible, or just assume integration tests ran)
	// We'll rely on the existing DB state from integration tests or migrate explicitly if needed.
	// Reusing database.MigrateDB is safest.
	if err := database.MigrateDB(perfDB); err != nil {
		log.Fatalf("failed to migrate: %v", err)
	}

	os.Exit(m.Run())
}

func TestLargeDatasetPerformance(t *testing.T) {
	if os.Getenv("RUN_PERF_TEST") != "true" {
		t.Skip("Skipping performance test. Set RUN_PERF_TEST=true to run.")
	}

	t.Log("Cleaning database...")
	if err := perfDB.Exec("TRUNCATE TABLE teldrive.files CASCADE").Error; err != nil {
		t.Fatal(err)
	}

	// 1. Bulk Insert 500k files using SQL generation
	fileCount := 500000
	userID := 123456789

	t.Logf("Inserting %d files...", fileCount)
	start := time.Now()

	// Insert diverse files
	err := perfDB.Exec(`
		INSERT INTO teldrive.files (id, name, type, mime_type, category, user_id, status, size, updated_at, created_at)
		SELECT
			gen_random_uuid(),
			'file_' || i,
			CASE WHEN i % 20 = 0 THEN 'folder' ELSE 'file' END,
			CASE
				WHEN i % 20 = 0 THEN 'drive/folder'
				WHEN i % 4 = 0 THEN 'image/jpeg'
				WHEN i % 4 = 1 THEN 'video/mp4'
				WHEN i % 4 = 2 THEN 'application/pdf'
				ELSE 'text/plain'
			END,
			CASE
				WHEN i % 20 = 0 THEN 'folder'
				WHEN i % 4 = 0 THEN 'image'
				WHEN i % 4 = 1 THEN 'video'
				WHEN i % 4 = 2 THEN 'document'
				ELSE 'other'
			END,
			?,
			'active',
			(random() * 1000000)::bigint,
			NOW(),
			NOW()
		FROM generate_series(1, ?) as i
	`, userID, fileCount).Error
	if err != nil {
		t.Fatal(err)
	}

	// Insert some pending deletion files (for status filtering tests)
	err = perfDB.Exec(`
		INSERT INTO teldrive.files (id, name, type, mime_type, category, user_id, status, size, updated_at, created_at)
		SELECT
			gen_random_uuid(),
			'deleted_' || i,
			'file',
			'application/octet-stream',
			'other',
			?,
			'pending_deletion',
			(random() * 1000000)::bigint,
			NOW(),
			NOW()
		FROM generate_series(1, 1000) as i
	`, userID).Error
	if err != nil {
		t.Fatal(err)
	}

	// Create a specific folder for Rclone parallel test
	rcloneFolderID := "019bc694-ade4-7778-9df4-3196b05b2841" // UUIDv7 example
	perfDB.Exec(`
		INSERT INTO teldrive.files (id, name, type, mime_type, category, user_id, status, size, updated_at, created_at)
		VALUES (?, 'RcloneTestFolder', 'folder', 'drive/folder', 'folder', ?, 'active', 0, NOW(), NOW())
	`, rcloneFolderID, userID)

	// Insert 10,000 files into this folder for Rclone test
	err = perfDB.Exec(`
		INSERT INTO teldrive.files (id, name, type, mime_type, category, user_id, status, size, parent_id, updated_at, created_at)
		SELECT
			gen_random_uuid(),
			'rclone_file_' || i,
			'file',
			'application/octet-stream',
			'other',
			?,
			'active',
			(random() * 1000000)::bigint,
			?,
			NOW(),
			NOW()
		FROM generate_series(1, 10000) as i
	`, userID, rcloneFolderID).Error
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Insertion took %v", time.Since(start))

	// Update statistics for accurate query planning
	perfDB.Exec("VACUUM ANALYZE teldrive.files")

	queries := []struct {
		name  string
		query string
		args  []any
	}{
		// --- Basic Listing & Sorting ---
		{
			name: "List Root Files (Limit 50, Sort UpdatedAt)",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'active' AND parent_id IS NULL
				ORDER BY updated_at DESC
				LIMIT 50
			`,
			args: []any{userID},
		},
		{
			name: "List Root Files (Limit 50, Sort Name ASC)",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'active' AND parent_id IS NULL
				ORDER BY name ASC
				LIMIT 50
			`,
			args: []any{userID},
		},
		{
			name: "List Root Files (Limit 50, Sort Size DESC)",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'active' AND parent_id IS NULL
				ORDER BY size DESC
				LIMIT 50
			`,
			args: []any{userID},
		},

		// --- Category Filtering ---
		{
			name: "Filter by Category (Image)",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'active' AND category = 'image'
				LIMIT 50
			`,
			args: []any{userID},
		},
		{
			name: "Filter by Category (Video) + Sort Size",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'active' AND category = 'video'
				ORDER BY size DESC
				LIMIT 50
			`,
			args: []any{userID},
		},
		{
			name: "Filter by Category (Document) + Pagination",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'active' AND category = 'document'
				ORDER BY updated_at DESC
				LIMIT 50 OFFSET 1000
			`,
			args: []any{userID},
		},

		// --- Type Filtering ---
		{
			name: "Filter by Type (Folder)",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'active' AND type = 'folder'
				LIMIT 50
			`,
			args: []any{userID},
		},
		{
			name: "Filter by Type (File) + Sort Name",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'active' AND type = 'file'
				ORDER BY name ASC
				LIMIT 50
			`,
			args: []any{userID},
		},

		// --- MimeType Filtering ---
		{
			name: "Filter by MimeType (PDF)",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'active' AND mime_type = 'application/pdf'
				LIMIT 50
			`,
			args: []any{userID},
		},

		// --- Status Filtering ---
		{
			name: "Filter by Status (PendingDeletion)",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'pending_deletion'
				LIMIT 50
			`,
			args: []any{userID},
		},

		// --- Global Sorting (All Files) ---
		{
			name: "Global Sort by Size (Large Files)",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'active'
				ORDER BY size DESC
				LIMIT 50
			`,
			args: []any{userID},
		},
		{
			name: "Global Sort by CreatedAt (Recent)",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'active'
				ORDER BY created_at DESC
				LIMIT 50
			`,
			args: []any{userID},
		},

		// --- Counting ---
		{
			name: "Count Files (Status Active)",
			query: `
				EXPLAIN ANALYZE SELECT count(*) FROM teldrive.files
				WHERE user_id = ? AND status = 'active'
			`,
			args: []any{userID},
		},
		{
			name: "Count by Category (Image)",
			query: `
				EXPLAIN ANALYZE SELECT count(*) FROM teldrive.files
				WHERE user_id = ? AND status = 'active' AND category = 'image'
			`,
			args: []any{userID},
		},
		{
			name: "Count by Type (Folder)",
			query: `
				EXPLAIN ANALYZE SELECT count(*) FROM teldrive.files
				WHERE user_id = ? AND status = 'active' AND type = 'folder'
			`,
			args: []any{userID},
		},

		// --- Search ---
		{
			name: "Search by Name (PGroonga Text - Function)",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'active'
				AND teldrive.clean_name(name) &@~ teldrive.clean_name(?)
				LIMIT 20
			`,
			args: []any{userID, "file_4999"},
		},
		{
			name: "Search by Name (PGroonga Regex)",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'active' AND name &~ ?
				LIMIT 20
			`,
			args: []any{userID, "file_4999"},
		},
		{
			name: "Search + Category (Image) + Deep Page",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'active' AND category = 'image' AND name &@~ ?
				LIMIT 50 OFFSET 5000
			`,
			args: []any{userID, "file_100"},
		},
		{
			name: "Search + Type (Folder) + Deep Page",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'active' AND type = 'folder' AND name &@~ ?
				LIMIT 20 OFFSET 5000
			`,
			args: []any{userID, "file_200"},
		},

		// --- Deep Pagination ---
		{
			name: "Deep Pagination (Offset 499,000)",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'active' AND parent_id IS NULL
				ORDER BY updated_at DESC
				LIMIT 50 OFFSET 499000
			`,
			args: []any{userID},
		},
		{
			name: "Deep Pagination (Deferred Join Optimization)",
			query: `
				EXPLAIN ANALYZE SELECT f.*
				FROM teldrive.files f
				JOIN (
					SELECT id FROM teldrive.files
					WHERE user_id = ? AND status = 'active' AND parent_id IS NULL
					ORDER BY updated_at DESC
					LIMIT 50 OFFSET 499000
				) sub ON f.id = sub.id
			`,
			args: []any{userID},
		},
		{
			name: "Deep Pagination + Category (Offset 50,000)",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'active' AND category = 'image'
				ORDER BY updated_at DESC
				LIMIT 50 OFFSET 50000
			`,
			args: []any{userID},
		},
		{
			name: "Deep Pagination + Type (Offset 10,000)",
			query: `
				EXPLAIN ANALYZE SELECT * FROM teldrive.files
				WHERE user_id = ? AND status = 'active' AND type = 'folder'
				ORDER BY updated_at DESC
				LIMIT 50 OFFSET 10000
			`,
			args: []any{userID},
		},
	}

	for _, q := range queries {
		t.Run(q.name, func(t *testing.T) {
			var result []string
			start := time.Now()
			// We use Raw to get the EXPLAIN output
			rows, err := perfDB.Raw(q.query, q.args...).Rows()
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			for rows.Next() {
				var line string
				rows.Scan(&line)
				result = append(result, line)
			}
			duration := time.Since(start)
			t.Logf("Query took: %v", duration)

			// Check for Index Scan in output
			usedIndex := false
			for _, line := range result {
				// t.Log(line) // Uncomment to see full plan
				if contains(line, "Index Scan") || contains(line, "Index Only Scan") || contains(line, "Bitmap Heap Scan") {
					usedIndex = true
				}
			}
			if !usedIndex {
				if duration < 50*time.Millisecond {
					t.Logf("Index not used, but query was fast (%v). Acceptable.", duration)
				} else {
					t.Errorf("Query did NOT use index! Plan:\n%v", result)
				}
			} else {
				t.Log("Index usage confirmed.")
			}
		})
	}
}

func TestRcloneParallel_UUIDv7(t *testing.T) {
	if os.Getenv("RUN_PERF_TEST") != "true" {
		t.Skip("Skipping performance test.")
	}

	userID := 123456789
	rcloneFolderID := "019bc694-ade4-7778-9df4-3196b05b2841"

	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(page int) {
			defer wg.Done()
			offset := page * 1000
			var res []models.File
			err := perfDB.Raw(`
				SELECT id, name, size, mime_type, type FROM teldrive.files
				WHERE user_id = ? AND status = 'active' AND parent_id = ?
				ORDER BY id DESC
				LIMIT 1000 OFFSET ?`, userID, rcloneFolderID, offset).Scan(&res).Error
			if err != nil {
				t.Errorf("Page %d failed: %v", page, err)
			}
		}(i)
	}
	wg.Wait()
	duration := time.Since(start)
	t.Logf("Rclone Parallel (8 pages, 8000 items) took: %v", duration)

	var plan []string
	perfDB.Raw(`
        EXPLAIN ANALYZE SELECT id, name, size, mime_type, type FROM teldrive.files
        WHERE user_id = ? AND status = 'active' AND parent_id = ?
        ORDER BY id DESC
        LIMIT 1000 OFFSET 7000`, userID, rcloneFolderID).Scan(&plan)

	usedIndex := false
	for _, line := range plan {
		if contains(line, "Index Only Scan") {
			usedIndex = true
		}
	}
	if !usedIndex {
		t.Logf("Warning: Rclone query did NOT use Index Only Scan. Plan: %v", plan)
	} else {
		t.Log("Confirmed: Index Only Scan used for Rclone query.")
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
