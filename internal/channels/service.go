package channels

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
)

var (
	ErrInvalidOwner     = errors.New("invalid channel owner")
	ErrInvalidChannel   = errors.New("channel does not belong to user")
	ErrNoSelected       = errors.New("no selected channel")
	ErrChannelFull      = errors.New("channel capacity reached")
	ErrAutoCreateOff    = errors.New("automatic channel creation is disabled")
	ErrChannelUnhealthy = errors.New("channel is unavailable")
)

// RemoteChannel is the minimum Telegram metadata required for durable storage.
type RemoteChannel struct {
	ID   int64
	Name string
}

// Creator performs Telegram-side channel lifecycle operations. Implementations
// are expected to add the configured upload bots before returning from Create.
type Creator interface {
	Create(ctx context.Context, userID int64, name string) (RemoteChannel, error)
	Delete(ctx context.Context, userID, channelID int64) error
}

type Config struct {
	PartLimit  int64
	AutoCreate bool
	NamePrefix string
}

type Service struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
	creator Creator
	config  Config
	now     func() time.Time
}

func NewService(pool *pgxpool.Pool, creator Creator, cfg Config) *Service {
	if cfg.NamePrefix == "" {
		cfg.NamePrefix = "storage"
	}
	return &Service{
		pool:    pool,
		queries: sqlcgen.New(pool),
		creator: creator,
		config:  cfg,
		now:     time.Now,
	}
}

// Resolve returns an owned channel suitable for one more uploaded part.
// Explicit channel requests never roll over silently.
func (s *Service) Resolve(ctx context.Context, userID, requestedChannelID int64) (int64, error) {
	if userID <= 0 {
		return 0, ErrInvalidOwner
	}
	if requestedChannelID != 0 {
		channel, err := s.queries.GetChannelForUser(ctx, sqlcgen.GetChannelForUserParams{
			UserID:    userID,
			ChannelID: requestedChannelID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrInvalidChannel
		}
		if err != nil {
			return 0, fmt.Errorf("get requested channel: %w", err)
		}
		if channel.Health == sqlcgen.ChannelHealthUnavailable {
			return 0, ErrChannelUnhealthy
		}
		full, err := s.limitReached(ctx, requestedChannelID)
		if err != nil {
			return 0, err
		}
		if full {
			return 0, ErrChannelFull
		}
		return requestedChannelID, nil
	}

	selected, err := s.queries.GetSelectedChannel(ctx, userID)
	if err == nil {
		if selected.Health != sqlcgen.ChannelHealthUnavailable {
			full, countErr := s.limitReached(ctx, selected.ChannelID)
			if countErr != nil {
				return 0, countErr
			}
			if !full {
				return selected.ChannelID, nil
			}
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("get selected channel: %w", err)
	}

	if !s.config.AutoCreate {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNoSelected
		}
		return 0, ErrAutoCreateOff
	}
	return s.rollover(ctx, userID)
}

func (s *Service) limitReached(ctx context.Context, channelID int64) (bool, error) {
	return s.limitReachedWith(ctx, s.queries, channelID)
}

func (s *Service) limitReachedWith(ctx context.Context, queries *sqlcgen.Queries, channelID int64) (bool, error) {
	if s.config.PartLimit <= 0 {
		return false, nil
	}
	count, err := queries.CountChannelStoredMessages(ctx, channelID)
	if err != nil {
		return false, fmt.Errorf("count channel parts: %w", err)
	}
	return count >= s.config.PartLimit, nil
}

func (s *Service) rollover(ctx context.Context, userID int64) (channelID int64, err error) {
	if s.creator == nil {
		return 0, errors.New("Telegram channel creator is not configured")
	}
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return 0, fmt.Errorf("acquire rollover connection: %w", err)
	}
	defer conn.Release()

	lockID := advisoryLockID(userID)
	lockedQueries := sqlcgen.New(conn)
	if err := lockedQueries.AcquireAdvisoryLock(ctx, lockID); err != nil {
		return 0, fmt.Errorf("acquire channel rollover lock: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, unlockErr := lockedQueries.ReleaseAdvisoryLock(unlockCtx, lockID); unlockErr != nil && err == nil {
			err = fmt.Errorf("release channel rollover lock: %w", unlockErr)
		}
	}()

	// Another process may have completed rollover while this caller waited.
	selected, selectedErr := lockedQueries.GetSelectedChannel(ctx, userID)
	if selectedErr == nil && selected.Health != sqlcgen.ChannelHealthUnavailable {
		full, countErr := s.limitReachedWith(ctx, lockedQueries, selected.ChannelID)
		if countErr != nil {
			return 0, countErr
		}
		if !full {
			return selected.ChannelID, nil
		}
	} else if selectedErr != nil && !errors.Is(selectedErr, pgx.ErrNoRows) {
		return 0, fmt.Errorf("recheck selected channel: %w", selectedErr)
	}

	name := fmt.Sprintf("%s_%s", strings.TrimSpace(s.config.NamePrefix), s.now().UTC().Format("20060102_150405"))
	remote, err := s.creator.Create(ctx, userID, name)
	if err != nil {
		return 0, fmt.Errorf("create Telegram channel: %w", err)
	}
	if remote.ID == 0 {
		return 0, errors.New("Telegram returned an empty channel id")
	}
	if strings.TrimSpace(remote.Name) == "" {
		remote.Name = name
	}

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		s.compensateDelete(userID, remote.ID)
		return 0, fmt.Errorf("begin channel rollover transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	q := lockedQueries.WithTx(tx)
	if err := q.ClearSelectedChannel(ctx, userID); err != nil {
		s.compensateDelete(userID, remote.ID)
		return 0, fmt.Errorf("clear selected channel: %w", err)
	}
	if _, err := q.CreateChannel(ctx, sqlcgen.CreateChannelParams{
		ChannelID: remote.ID,
		UserID:    userID,
		Name:      remote.Name,
	}); err != nil {
		s.compensateDelete(userID, remote.ID)
		return 0, fmt.Errorf("create channel record: %w", err)
	}
	if _, err := q.SelectChannel(ctx, sqlcgen.SelectChannelParams{
		UserID:    userID,
		ChannelID: remote.ID,
	}); err != nil {
		s.compensateDelete(userID, remote.ID)
		return 0, fmt.Errorf("select channel record: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		s.compensateDelete(userID, remote.ID)
		return 0, fmt.Errorf("commit channel rollover: %w", err)
	}
	return remote.ID, nil
}

func (s *Service) compensateDelete(userID, channelID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = s.creator.Delete(ctx, userID, channelID)
}

func advisoryLockID(userID int64) int64 {
	var input [8]byte
	binary.BigEndian.PutUint64(input[:], uint64(userID))
	digest := sha256.Sum256(append([]byte("teldrive/channel-rollover/"), input[:]...))
	return int64(binary.BigEndian.Uint64(digest[:8]))
}
