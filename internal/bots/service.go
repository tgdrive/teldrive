package bots

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
	"github.com/tgdrive/teldrive/v2/internal/secureblob"
)

var (
	ErrInvalidInput = errors.New("invalid bot input")
	ErrNotFound     = errors.New("bot not found")
	ErrNotBot       = errors.New("Telegram credential does not belong to a bot")
)

type Identity struct {
	ID       int64
	Username string
}

type Verifier interface {
	Verify(context.Context, string) (Identity, error)
}

type Provisioner interface {
	ProvisionBot(context.Context, int64, Identity) error
}

type ListInput struct {
	UserID         int64
	AfterCreatedAt *time.Time
	AfterBotID     *int64
	Limit          int32
}

type Service struct {
	pool     *pgxpool.Pool
	queries  *sqlcgen.Queries
	cipher   *secureblob.Cipher
	verifier Verifier
}

func NewService(pool *pgxpool.Pool, cipher *secureblob.Cipher, verifier Verifier) (*Service, error) {
	if pool == nil || cipher == nil || verifier == nil {
		return nil, ErrInvalidInput
	}
	return &Service{pool: pool, queries: sqlcgen.New(pool), cipher: cipher, verifier: verifier}, nil
}

func TokenBotID(token string) (int64, error) {
	token = strings.TrimSpace(token)
	prefix, _, ok := strings.Cut(token, ":")
	if !ok || prefix == "" {
		return 0, ErrInvalidInput
	}
	botID, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil || botID <= 0 {
		return 0, ErrInvalidInput
	}
	return botID, nil
}

func (s *Service) Create(ctx context.Context, userID int64, token string) (*sqlcgen.Bot, error) {
	token = strings.TrimSpace(token)
	if userID <= 0 || token == "" {
		return nil, ErrInvalidInput
	}
	rows, err := s.InsertPending(ctx, userID, []string{token})
	if err != nil {
		return nil, err
	}
	botID, err := TokenBotID(token)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return s.VerifyPending(ctx, userID, botID)
	}
	return s.VerifyPending(ctx, userID, rows[0].BotID)
}
func (s *Service) InsertPending(ctx context.Context, userID int64, tokens []string) ([]*sqlcgen.Bot, error) {
	if userID <= 0 || len(tokens) == 0 {
		return nil, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin pending bot insert: %w", err)
	}
	defer tx.Rollback(ctx)
	queries := s.queries.WithTx(tx)
	rows := make([]*sqlcgen.Bot, 0, len(tokens))
	seen := make(map[int64]struct{}, len(tokens))
	for _, raw := range tokens {
		token := strings.TrimSpace(raw)
		botID, parseErr := TokenBotID(token)
		if parseErr != nil {
			return nil, parseErr
		}
		if _, exists := seen[botID]; exists {
			continue
		}
		seen[botID] = struct{}{}
		ciphertext, sealErr := s.cipher.Seal("bot-token", []byte(token))
		if sealErr != nil {
			return nil, sealErr
		}
		inserted, insertErr := queries.InsertPendingBot(ctx, sqlcgen.InsertPendingBotParams{
			BotID: botID, UserID: userID, TokenCiphertext: ciphertext,
		})
		if insertErr != nil {
			return nil, fmt.Errorf("insert pending bot %d: %w", botID, insertErr)
		}
		if inserted == 0 {
			continue
		}
		row, getErr := queries.GetBot(ctx, sqlcgen.GetBotParams{UserID: userID, BotID: botID})
		if getErr != nil {
			return nil, fmt.Errorf("load pending bot %d: %w", botID, getErr)
		}
		rows = append(rows, row)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit pending bots: %w", err)
	}
	return rows, nil
}

func (s *Service) VerifyPending(ctx context.Context, userID, botID int64) (*sqlcgen.Bot, error) {
	if userID <= 0 || botID <= 0 {
		return nil, ErrInvalidInput
	}
	row, err := s.queries.GetBot(ctx, sqlcgen.GetBotParams{UserID: userID, BotID: botID})
	if err != nil {
		return nil, fmt.Errorf("load pending bot: %w", err)
	}
	token, err := s.cipher.Open("bot-token", row.TokenCiphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt bot token: %w", err)
	}
	identity, err := s.verifier.Verify(ctx, string(token))
	if err != nil {
		return nil, err
	}
	if identity.ID != botID || strings.TrimSpace(identity.Username) == "" {
		return nil, ErrNotBot
	}
	activated, err := s.queries.ActivateBot(ctx, sqlcgen.ActivateBotParams{
		Username: dbtypes.OptionalText(nonEmpty(identity.Username)), UserID: userID, BotID: botID,
	})
	if err != nil {
		return nil, fmt.Errorf("activate bot: %w", err)
	}
	return activated, nil
}

func (s *Service) MarkProvisionFailure(ctx context.Context, userID, botID int64, cause error) error {
	message := "bot provisioning failed"
	if cause != nil {
		message = cause.Error()
	}
	_, err := s.queries.MarkBotProvisionFailure(ctx, sqlcgen.MarkBotProvisionFailureParams{
		LastError: dbtypes.OptionalText(&message), UserID: userID, BotID: botID,
	})
	return err
}

func (s *Service) HasExistingChannels(ctx context.Context, userID int64) (bool, error) {
	if userID <= 0 {
		return false, ErrInvalidInput
	}
	rows, err := s.queries.ListChannels(ctx, sqlcgen.ListChannelsParams{UserID: userID, PageSize: 1})
	if err != nil {
		return false, fmt.Errorf("list channels for bot provisioning: %w", err)
	}
	return len(rows) > 0, nil
}

func (s *Service) List(ctx context.Context, in ListInput) ([]*sqlcgen.Bot, error) {
	if in.UserID <= 0 {
		return nil, ErrInvalidInput
	}
	if in.Limit <= 0 {
		in.Limit = 100
	}
	if in.Limit > 200 {
		in.Limit = 200
	}
	var afterID pgtype.Int8
	if in.AfterBotID != nil {
		afterID = dbtypes.Int8(*in.AfterBotID)
	}
	rows, err := s.queries.ListBots(ctx, sqlcgen.ListBotsParams{
		UserID: in.UserID, AfterCreatedAt: dbtypes.OptionalTime(in.AfterCreatedAt),
		AfterBotID: afterID, PageSize: in.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list bots: %w", err)
	}
	return rows, nil
}

func (s *Service) Delete(ctx context.Context, userID, botID int64) error {
	if userID <= 0 || botID <= 0 {
		return ErrInvalidInput
	}
	count, err := s.queries.DeleteBot(ctx, sqlcgen.DeleteBotParams{UserID: userID, BotID: botID})
	if err != nil {
		return fmt.Errorf("delete bot: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func nonEmpty(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
