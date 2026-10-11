package botgateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/gotd/td/session"

	"github.com/tgdrive/teldrive/v2/internal/telethonsession"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
)

const telethonSessionVersion byte = '1'

type botSessionStorage struct {
	queries *sqlcgen.Queries
	cipher  *secureblob.Cipher
	userID  int64
	botID   int64
	stored  []byte
}

func (s *botSessionStorage) HasSession() bool {
	return s != nil && len(s.stored) > 0
}

func (s *botSessionStorage) LoadSession(ctx context.Context) ([]byte, error) {
	if !s.HasSession() {
		return nil, session.ErrNotFound
	}
	plain, err := s.cipher.Open("bot-session", s.stored)
	if err != nil {
		return nil, fmt.Errorf("decrypt bot session: %w", err)
	}
	return telethonsession.DecodeToGotd(ctx, string(plain))
}

func (s *botSessionStorage) StoreSession(ctx context.Context, raw []byte) error {
	if s == nil || s.queries == nil || s.cipher == nil || s.userID <= 0 || s.botID <= 0 || len(raw) == 0 {
		return errors.New("invalid bot session storage")
	}
	encoded, err := telethonsession.EncodeGotd(ctx, raw)
	if err != nil {
		return err
	}
	ciphertext, err := s.cipher.Seal("bot-session", []byte(encoded))
	if err != nil {
		return fmt.Errorf("encrypt bot session: %w", err)
	}
	count, err := s.queries.UpdateBotSession(ctx, sqlcgen.UpdateBotSessionParams{
		Session: ciphertext,
		UserID:  s.userID,
		BotID:   s.botID,
	})
	if err != nil {
		return fmt.Errorf("persist bot session: %w", err)
	}
	if count == 0 {
		return errors.New("bot session row no longer exists")
	}
	s.stored = append(s.stored[:0], ciphertext...)
	return nil
}
