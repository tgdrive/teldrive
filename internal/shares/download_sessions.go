package shares

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
)

const DownloadSessionTTL = 15 * time.Minute

type DownloadSession struct {
	Token     string
	ExpiresAt time.Time
}

// ContentRequest credentials authorize content only, never listing or editing.
// A nil FileID selects the root file; a non-nil ID must belong to the shared tree.
type ContentRequest struct {
	Token    string
	Password string
	Session  string
	FileID   *uuid.UUID
}

func (s *Service) CreateDownloadSession(ctx context.Context, token, password string) (*DownloadSession, error) {
	resolved, err := s.Resolve(ctx, token, password)
	if err != nil {
		return nil, err
	}
	secret, hash, err := s.newToken()
	if err != nil {
		return nil, err
	}
	expiresAt := s.now().Add(DownloadSessionTTL)
	if resolved.Share.ExpiresAt.Valid && resolved.Share.ExpiresAt.Time.Before(expiresAt) {
		expiresAt = resolved.Share.ExpiresAt.Time
	}
	if err := s.queries.CreateShareDownloadSession(ctx, sqlcgen.CreateShareDownloadSessionParams{
		TokenHash: hash, ShareID: resolved.Share.ID, PasswordHash: resolved.Share.PasswordHash,
		ExpiresAt: dbtypes.Time(expiresAt),
	}); err != nil {
		return nil, fmt.Errorf("create share download session: %w", err)
	}
	return &DownloadSession{Token: secret, ExpiresAt: expiresAt}, nil
}

func (s *Service) ResolveContent(ctx context.Context, in ContentRequest) (*Public, error) {
	if in.FileID != nil && *in.FileID == uuid.Nil {
		return nil, ErrNotFound
	}
	resolved, err := s.resolve(ctx, in.Token, in.Password, in.Session)
	if err != nil {
		return nil, err
	}
	if in.FileID == nil {
		return resolved, nil
	}
	return s.resolveFile(ctx, resolved, *in.FileID)
}

func (s *Service) ReserveContentDownload(ctx context.Context, in ContentRequest) (*Public, error) {
	resolved, err := s.ResolveContent(ctx, in)
	if err != nil {
		return nil, err
	}
	// Use the same atomic reservation as header-authorized downloads, even when
	// the browser uses a cookie. Failed streams consume the reservation as before.
	if _, err := s.queries.IncrementShareDownloadCount(ctx, resolved.Share.ID); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrExpired
	} else if err != nil {
		return nil, fmt.Errorf("reserve share download: %w", err)
	}
	return resolved, nil
}
