package bots

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
)

var (
	ErrManagedUnavailable  = errors.New("managed bot creation is unavailable")
	ErrManagerPermission   = errors.New("enable Bot Management Mode for the manager in BotFather")
	ErrBotUsernameOccupied = errors.New("bot username is already occupied by another bot")
	ErrBotUsernameInvalid  = errors.New("bot username must contain 5-32 letters, digits or underscores and end in bot")
	ErrBotCreateLimit      = errors.New("Telegram bot creation limit reached")
	ErrBotFloodWait        = errors.New("Telegram requires waiting before another bot request")
	ErrManagedTelegram     = errors.New("Telegram could not complete the managed bot request")
	botUsername            = regexp.MustCompile(`(?i)^[a-z0-9_]{2,29}bot$`)
)

type ManagedIdentity struct {
	Identity
	Token     string
	Recovered bool
}

type ManagedGateway interface {
	Manager(context.Context, string) (Identity, error)
	CheckUsername(context.Context, int64, string) error
	CreateManaged(context.Context, int64, string, string, string) (ManagedIdentity, error)
}

func NormalizeBotUsername(value string) (string, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "@")
	if !botUsername.MatchString(value) {
		return "", ErrBotUsernameInvalid
	}
	return value, nil
}

func (s *Service) ConfigureManaged(gateway ManagedGateway) *Service {
	s.managed = gateway
	return s
}

func (s *Service) managerToken(ctx context.Context, userID, botID int64) (string, error) {
	if s.managed == nil {
		return "", ErrManagedUnavailable
	}
	if userID <= 0 || botID <= 0 {
		return "", ErrInvalidInput
	}
	row, err := s.queries.GetBot(ctx, sqlcgen.GetBotParams{UserID: userID, BotID: botID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	token, err := s.cipher.Open("bot-token", row.TokenCiphertext)
	return string(token), err
}

func (s *Service) Manager(ctx context.Context, userID, botID int64) (Identity, error) {
	token, err := s.managerToken(ctx, userID, botID)
	if err != nil {
		return Identity{}, err
	}
	identity, err := s.managed.Manager(ctx, token)
	if err == nil && identity.ID != botID {
		return Identity{}, ErrNotBot
	}
	return identity, err
}

func (s *Service) CheckManagedUsername(ctx context.Context, userID int64, username string) error {
	if s.managed == nil {
		return ErrManagedUnavailable
	}
	username, err := NormalizeBotUsername(username)
	if err != nil {
		return err
	}
	return s.managed.CheckUsername(ctx, userID, username)
}

func (s *Service) CreateManaged(ctx context.Context, userID, managerID int64, name, username string) (*sqlcgen.Bot, bool, error) {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 64 {
		return nil, false, ErrInvalidInput
	}
	username, err := NormalizeBotUsername(username)
	if err != nil {
		return nil, false, err
	}
	token, err := s.managerToken(ctx, userID, managerID)
	if err != nil {
		return nil, false, err
	}
	created, err := s.managed.CreateManaged(ctx, userID, token, name, username)
	if err != nil {
		return nil, false, err
	}
	botID, err := TokenBotID(created.Token)
	if err != nil || botID != created.ID || !strings.EqualFold(created.Username, username) {
		return nil, false, ErrNotBot
	}
	_, err = s.InsertPending(ctx, userID, []string{created.Token})
	if err != nil {
		return nil, false, err
	}
	row, err := s.saveIdentity(ctx, userID, created.Identity)
	return row, created.Recovered, err
}
