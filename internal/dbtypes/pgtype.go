package dbtypes

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func UUID(v uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: v, Valid: true}
}

func OptionalUUID(v *uuid.UUID) pgtype.UUID {
	if v == nil {
		return pgtype.UUID{}
	}
	return UUID(*v)
}

func Time(v time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: v, Valid: true}
}

func OptionalTime(v *time.Time) pgtype.Timestamptz {
	if v == nil {
		return pgtype.Timestamptz{}
	}
	return Time(*v)
}

func Text(v string) pgtype.Text {
	return pgtype.Text{String: v, Valid: true}
}

func OptionalText(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return Text(*v)
}

func Int4(v int32) pgtype.Int4 {
	return pgtype.Int4{Int32: v, Valid: true}
}

func OptionalInt4(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return Int4(*v)
}

func Int8(v int64) pgtype.Int8 {
	return pgtype.Int8{Int64: v, Valid: true}
}

func OptionalInt8(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return Int8(*v)
}

func GoogleUUID(v pgtype.UUID) (uuid.UUID, bool) {
	if !v.Valid {
		return uuid.Nil, false
	}
	return uuid.UUID(v.Bytes), true
}
