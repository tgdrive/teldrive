package uploads

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
)

func TestExpectedPartCount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		size, partSize int64
		want           int64
	}{
		{0, 4, 0},
		{1, 4, 1},
		{4, 4, 1},
		{5, 4, 2},
		{10, 4, 3},
	}
	for _, tt := range cases {
		if got := expectedPartCount(tt.size, tt.partSize); got != tt.want {
			t.Fatalf("expectedPartCount(%d, %d) = %d, want %d", tt.size, tt.partSize, got, tt.want)
		}
	}
}

func TestValidatePartShape(t *testing.T) {
	t.Parallel()
	session := &sqlcgen.UploadSession{ExpectedSize: 10, PartSize: 4}
	valid := []struct {
		part int32
		size int64
	}{{1, 4}, {2, 4}, {3, 2}}
	for _, tt := range valid {
		if err := validatePartShape(session, tt.part, tt.size); err != nil {
			t.Fatalf("part %d size %d rejected: %v", tt.part, tt.size, err)
		}
	}
	invalid := []struct {
		part int32
		size int64
	}{{0, 4}, {1, 3}, {3, 4}, {4, 0}}
	for _, tt := range invalid {
		if err := validatePartShape(session, tt.part, tt.size); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("part %d size %d error = %v", tt.part, tt.size, err)
		}
	}
	if err := validatePartShape(&sqlcgen.UploadSession{}, 1, 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero-size session error = %v", err)
	}
}

func TestOptionalTextEqual(t *testing.T) {
	t.Parallel()
	value := "hash"
	if !optionalTextEqual(pgtype.Text{}, nil) {
		t.Fatal("unset values should match")
	}
	if optionalTextEqual(pgtype.Text{}, &value) {
		t.Fatal("missing stored value should not match")
	}
	if !optionalTextEqual(pgtype.Text{String: value, Valid: true}, &value) {
		t.Fatal("equal values should match")
	}
	other := "other"
	if optionalTextEqual(pgtype.Text{String: value, Valid: true}, &other) {
		t.Fatal("different values should not match")
	}
}
