package catalog

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
)

type UpdateInput struct {
	UserID             int64
	FileID             uuid.UUID
	ExpectedGeneration *int64
	Name               *string
	ModTime            *time.Time
}

func (s *Service) Update(ctx context.Context, in UpdateInput) (*sqlcgen.File, error) {
	if in.UserID <= 0 {
		return nil, ErrInvalidOwner
	}
	if in.FileID == uuid.Nil || (in.Name == nil && in.ModTime == nil) {
		return nil, ErrInvalidName
	}
	var name pgtype.Text
	var normalized pgtype.Text
	if in.Name != nil {
		display, folded, err := NormalizeName(*in.Name)
		if err != nil {
			return nil, err
		}
		name = dbtypes.Text(display)
		normalized = dbtypes.Text(folded)
	}
	file, err := s.queries.UpdateFileMetadata(ctx, sqlcgen.UpdateFileMetadataParams{
		Name:               name,
		NormalizedName:     normalized,
		ModTime:            dbtypes.OptionalTime(in.ModTime),
		FileID:             dbtypes.UUID(in.FileID),
		UserID:             in.UserID,
		ExpectedGeneration: dbtypes.OptionalInt8(in.ExpectedGeneration),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		if in.ExpectedGeneration != nil {
			return nil, ErrPrecondition
		}
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, classifyWriteError("update file", err)
	}
	s.invalidateFile(ctx, in.UserID, in.FileID)
	return file, nil
}
