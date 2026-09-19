package jobs

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
)

func mustTestFileUUID(id string) pgtype.UUID {
	return dbtypes.UUID(uuid.MustParse(id))
}

func TestOrphanCleanupOutputJSONShape(t *testing.T) {
	t.Parallel()
	cutoff := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	completed := cutoff.Add(time.Minute)
	raw, err := json.Marshal(OrphanCleanupOutput{Channels: 2, Scanned: 150, Deleted: 3, Cutoff: cutoff, CompletedAt: completed})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for _, key := range []string{"channels", "scanned", "deleted", "cutoff", "completedAt", "brokenFiles", "brokenTotal", "brokenTruncated"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("output JSON = %s, missing key %q", raw, key)
		}
	}
	if decoded["channels"] != 2.0 || decoded["scanned"] != 150.0 || decoded["deleted"] != 3.0 {
		t.Fatalf("output counters = %s, want channels/scanned/deleted 2/150/3", raw)
	}
}

func TestFindBrokenFiles(t *testing.T) {
	t.Parallel()
	rows := []*sqlcgen.ListChannelReferencedPartsRow{
		{MessageID: 10, FileID: mustTestFileUUID("11111111-1111-1111-1111-111111111111"), FileName: "b.bin", FileSize: pgtype.Int8{Int64: 20, Valid: true}},
		{MessageID: 11, FileID: mustTestFileUUID("11111111-1111-1111-1111-111111111111"), FileName: "b.bin", FileSize: pgtype.Int8{Int64: 20, Valid: true}},
		{MessageID: 12, FileID: mustTestFileUUID("22222222-2222-2222-2222-222222222222"), FileName: "a.bin", FileSize: pgtype.Int8{Int64: 10, Valid: true}},
		{MessageID: 13, FileID: mustTestFileUUID("22222222-2222-2222-2222-222222222222"), FileName: "a.bin", FileSize: pgtype.Int8{Int64: 10, Valid: true}},
	}
	seen := map[int64]struct{}{10: {}, 12: {}}
	broken := findBrokenFiles(seen, rows, 9001)
	if len(broken) != 2 {
		t.Fatalf("findBrokenFiles() len = %d, want 2", len(broken))
	}
	if broken[0].Name != "a.bin" || broken[1].Name != "b.bin" {
		t.Fatalf("broken files not sorted by name: %+v", broken)
	}
	if len(broken[0].MissingMessageIDs) != 1 || broken[0].MissingMessageIDs[0] != 13 {
		t.Fatalf("a.bin missing = %v, want [13]", broken[0].MissingMessageIDs)
	}
	if broken[0].FileID != "22222222-2222-2222-2222-222222222222" || broken[0].Size != 10 || broken[0].ChannelID != 9001 {
		t.Fatalf("a.bin entry = %+v", broken[0])
	}
	if got := findBrokenFiles(map[int64]struct{}{10: {}, 11: {}, 12: {}, 13: {}}, rows, 9001); len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestLimitBrokenFilesSize(t *testing.T) {
	t.Parallel()
	output := OrphanCleanupOutput{
		Channels: 1, Scanned: 4, Deleted: 0,
		BrokenFiles: []BrokenFile{{FileID: "22222222-2222-2222-2222-222222222222", Name: "a.bin", Size: 10, ChannelID: 9001, MissingMessageIDs: []int64{13}}},
		BrokenTotal: 1,
	}
	if got := limitBrokenFilesSize(output, 1<<20); got.BrokenTruncated || len(got.BrokenFiles) != 1 {
		t.Fatalf("under budget: truncated = %v, len = %d, want false/1", got.BrokenTruncated, len(got.BrokenFiles))
	}
	if got := limitBrokenFilesSize(output, 10); !got.BrokenTruncated || len(got.BrokenFiles) != 0 || got.BrokenTotal != 1 {
		t.Fatalf("over budget: truncated = %v, len = %d, total = %d, want true/0/1", got.BrokenTruncated, len(got.BrokenFiles), got.BrokenTotal)
	}
	empty := OrphanCleanupOutput{BrokenFiles: []BrokenFile{}, BrokenTotal: 0}
	if got := limitBrokenFilesSize(empty, 10); got.BrokenTruncated {
		t.Fatalf("empty list must never truncate")
	}
}
