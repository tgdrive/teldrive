package catalog

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
)

func TestBulkCatalogHelpers(t *testing.T) {
	t.Parallel()

	if base, extension := splitCatalogName("archive.tar.gz"); base != "archive.tar" || extension != ".gz" {
		t.Fatalf("splitCatalogName() = %q, %q", base, extension)
	}
	for _, name := range []string{"README", ".hidden", "trailing."} {
		if base, extension := splitCatalogName(name); base != name || extension != "" {
			t.Fatalf("splitCatalogName(%q) = %q, %q", name, base, extension)
		}
	}

	bounded := boundedCatalogName(strings.Repeat("界", 300), ".txt", " (1)")
	if len([]rune(bounded)) != 255 || !strings.HasSuffix(bounded, " (1).txt") {
		t.Fatalf("boundedCatalogName() produced %d runes with suffix %q", len([]rune(bounded)), bounded[len(bounded)-12:])
	}
	oversizedExtension := boundedCatalogName("base", strings.Repeat("x", 300), " (2)")
	if len([]rune(oversizedExtension)) != len([]rune("base (2)")) || oversizedExtension != "base (2)" {
		t.Fatalf("oversized extension result = %q", oversizedExtension)
	}

	parent := uuid.New()
	rootLock := catalogDestinationLockID(1001, nil)
	parentLock := catalogDestinationLockID(1001, &parent)
	if rootLock == parentLock || rootLock != catalogDestinationLockID(1001, nil) || parentLock != catalogDestinationLockID(1001, &parent) {
		t.Fatalf("lock IDs are not stable/distinct: root=%d parent=%d", rootLock, parentLock)
	}

	ids := []uuid.UUID{uuid.New(), uuid.New()}
	converted := pgUUIDs(ids)
	if len(converted) != len(ids) {
		t.Fatalf("pgUUIDs() length = %d", len(converted))
	}
	for index, id := range ids {
		got, ok := dbtypes.GoogleUUID(converted[index])
		if !ok || got != id {
			t.Fatalf("pgUUIDs()[%d] = %v, %t", index, got, ok)
		}
	}

	if id, ok := fileUUID(nil); ok || id != uuid.Nil {
		t.Fatalf("fileUUID(nil) = %v, %t", id, ok)
	}
	if id, ok := fileUUID(&sqlcgen.File{}); ok || id != uuid.Nil {
		t.Fatalf("fileUUID(invalid) = %v, %t", id, ok)
	}
	validID := uuid.New()
	validFile := &sqlcgen.File{ID: dbtypes.UUID(validID)}
	if id, ok := fileUUID(validFile); !ok || id != validID {
		t.Fatalf("fileUUID(valid) = %v, %t", id, ok)
	}

	first := uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff")
	second := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	stable := StableIDs([]*sqlcgen.File{
		{ID: dbtypes.UUID(first)}, nil, {ID: pgtype.UUID{}}, {ID: dbtypes.UUID(second)},
	})
	if len(stable) != 2 || stable[0] != second || stable[1] != first {
		t.Fatalf("StableIDs() = %v", stable)
	}
}

func TestFileCursorValueVariants(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	updatedAt := time.Date(2026, 7, 2, 12, 34, 56, 789, time.UTC)
	file := &sqlcgen.File{
		ID:             dbtypes.UUID(id),
		NormalizedName: "Name",
		Size:           pgtype.Int8{Int64: 42, Valid: true},
		UpdatedAt:      pgtype.Timestamptz{Time: updatedAt, Valid: true},
	}
	if got := FileCursorValue(file, "id"); got != id.String() {
		t.Fatalf("id cursor = %q", got)
	}
	if got := FileCursorValue(file, "size"); got != "42" {
		t.Fatalf("size cursor = %q", got)
	}
	if got := FileCursorValue(file, "updatedAt"); got != updatedAt.Format(time.RFC3339Nano) {
		t.Fatalf("updated cursor = %q", got)
	}
	if got := FileCursorValue(file, "name"); got != "Name" {
		t.Fatalf("name cursor = %q", got)
	}
	if got := FileCursorValue(&sqlcgen.File{}, "size"); got != "-1" {
		t.Fatalf("invalid size cursor = %q", got)
	}
	if got := FileCursorValue(&sqlcgen.File{}, "id"); got != "" {
		t.Fatalf("invalid ID cursor = %q", got)
	}
	if got := FileCursorValue(nil, "name"); got != "" {
		t.Fatalf("nil cursor = %q", got)
	}
}
