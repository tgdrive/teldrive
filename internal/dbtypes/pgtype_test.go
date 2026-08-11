package dbtypes

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestConversions(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	if got := UUID(id); !got.Valid || uuid.UUID(got.Bytes) != id {
		t.Fatalf("UUID conversion mismatch: %#v", got)
	}
	if got := OptionalUUID(nil); got.Valid {
		t.Fatalf("nil OptionalUUID should be invalid: %#v", got)
	}
	if got, ok := GoogleUUID(UUID(id)); !ok || got != id {
		t.Fatalf("GoogleUUID() = %v, %v", got, ok)
	}
	if _, ok := GoogleUUID(OptionalUUID(nil)); ok {
		t.Fatal("invalid pgtype.UUID should not convert")
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	if got := Time(now); !got.Valid || !got.Time.Equal(now) {
		t.Fatalf("Time conversion mismatch: %#v", got)
	}
	if got := OptionalTime(nil); got.Valid {
		t.Fatalf("nil OptionalTime should be invalid: %#v", got)
	}
	if got := OptionalTime(&now); !got.Valid || !got.Time.Equal(now) {
		t.Fatalf("OptionalTime conversion mismatch: %#v", got)
	}

	text := "value"
	if got := Text(text); !got.Valid || got.String != text {
		t.Fatalf("Text conversion mismatch: %#v", got)
	}
	if got := OptionalText(nil); got.Valid {
		t.Fatalf("nil OptionalText should be invalid: %#v", got)
	}
	if got := OptionalText(&text); !got.Valid || got.String != text {
		t.Fatalf("OptionalText conversion mismatch: %#v", got)
	}

	i4 := int32(4)
	if got := Int4(i4); !got.Valid || got.Int32 != i4 {
		t.Fatalf("Int4 conversion mismatch: %#v", got)
	}
	if got := OptionalInt4(nil); got.Valid {
		t.Fatalf("nil OptionalInt4 should be invalid: %#v", got)
	}
	if got := OptionalInt4(&i4); !got.Valid || got.Int32 != i4 {
		t.Fatalf("OptionalInt4 conversion mismatch: %#v", got)
	}

	i8 := int64(8)
	if got := Int8(i8); !got.Valid || got.Int64 != i8 {
		t.Fatalf("Int8 conversion mismatch: %#v", got)
	}
	if got := OptionalInt8(nil); got.Valid {
		t.Fatalf("nil OptionalInt8 should be invalid: %#v", got)
	}
	if got := OptionalInt8(&i8); !got.Valid || got.Int64 != i8 {
		t.Fatalf("OptionalInt8 conversion mismatch: %#v", got)
	}
}
