package channels

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/dbtypes"
)

var (
	ErrSelectedChannel = errors.New("selected channel cannot be deleted")
	ErrChannelInUse    = errors.New("channel still contains referenced objects")
)

type ListInput struct {
	UserID         int64
	AfterCreatedAt *time.Time
	AfterChannelID *int64
	Limit          int32
}

func (s *Service) List(ctx context.Context, in ListInput) ([]*sqlcgen.Channel, error) {
	if in.UserID <= 0 {
		return nil, ErrInvalidOwner
	}
	if in.Limit <= 0 {
		in.Limit = 100
	}
	if in.Limit > 200 {
		in.Limit = 200
	}
	var afterID pgtype.Int8
	if in.AfterChannelID != nil {
		afterID = dbtypes.Int8(*in.AfterChannelID)
	}
	rows, err := s.queries.ListChannels(ctx, sqlcgen.ListChannelsParams{
		UserID: in.UserID, AfterCreatedAt: dbtypes.OptionalTime(in.AfterCreatedAt),
		AfterChannelID: afterID, PageSize: in.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	return rows, nil
}

func (s *Service) Create(ctx context.Context, userID int64, name string, selected bool) (*sqlcgen.Channel, error) {
	if userID <= 0 || s.creator == nil {
		return nil, ErrInvalidOwner
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = fmt.Sprintf("%s_%s", strings.TrimSpace(s.config.NamePrefix), s.now().UTC().Format("20060102_150405"))
	}
	remote, err := s.creator.Create(ctx, userID, name)
	if err != nil {
		return nil, fmt.Errorf("create Telegram channel: %w", err)
	}
	if remote.ID == 0 {
		return nil, errors.New("Telegram returned an empty channel id")
	}
	if strings.TrimSpace(remote.Name) == "" {
		remote.Name = name
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.compensateDelete(userID, remote.ID)
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	if selected {
		if err := q.ClearSelectedChannel(ctx, userID); err != nil {
			s.compensateDelete(userID, remote.ID)
			return nil, fmt.Errorf("clear selected channel: %w", err)
		}
	}
	row, err := q.CreateChannel(ctx, sqlcgen.CreateChannelParams{ChannelID: remote.ID, UserID: userID, Name: remote.Name})
	if err != nil {
		s.compensateDelete(userID, remote.ID)
		return nil, fmt.Errorf("create channel record: %w", err)
	}
	if selected {
		row, err = q.SelectChannel(ctx, sqlcgen.SelectChannelParams{UserID: userID, ChannelID: remote.ID})
		if err != nil {
			s.compensateDelete(userID, remote.ID)
			return nil, fmt.Errorf("select created channel: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		s.compensateDelete(userID, remote.ID)
		return nil, fmt.Errorf("commit created channel: %w", err)
	}
	return row, nil
}

func (s *Service) Select(ctx context.Context, userID, channelID int64) (*sqlcgen.Channel, error) {
	if userID <= 0 || channelID == 0 {
		return nil, ErrInvalidChannel
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin channel selection: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	channel, err := q.GetChannelForUser(ctx, sqlcgen.GetChannelForUserParams{UserID: userID, ChannelID: channelID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidChannel
	}
	if err != nil {
		return nil, fmt.Errorf("get channel for selection: %w", err)
	}
	if channel.Health == sqlcgen.ChannelHealthUnavailable {
		return nil, ErrChannelUnhealthy
	}
	if err := q.ClearSelectedChannel(ctx, userID); err != nil {
		return nil, fmt.Errorf("clear selected channel: %w", err)
	}
	channel, err = q.SelectChannel(ctx, sqlcgen.SelectChannelParams{UserID: userID, ChannelID: channelID})
	if err != nil {
		return nil, fmt.Errorf("select channel: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit channel selection: %w", err)
	}
	return channel, nil
}

func (s *Service) Delete(ctx context.Context, userID, channelID int64) error {
	if userID <= 0 || channelID == 0 || s.creator == nil {
		return ErrInvalidChannel
	}
	channel, err := s.queries.GetChannelForUser(ctx, sqlcgen.GetChannelForUserParams{UserID: userID, ChannelID: channelID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidChannel
	}
	if err != nil {
		return fmt.Errorf("get channel for deletion: %w", err)
	}
	if channel.Selected {
		return ErrSelectedChannel
	}
	references, err := s.queries.CountChannelReferences(ctx, channelID)
	if err != nil {
		return fmt.Errorf("count channel references: %w", err)
	}
	if references > 0 {
		return ErrChannelInUse
	}
	if err := s.creator.Delete(ctx, userID, channelID); err != nil {
		return fmt.Errorf("delete Telegram channel: %w", err)
	}
	count, err := s.queries.DeleteChannel(ctx, sqlcgen.DeleteChannelParams{UserID: userID, ChannelID: channelID})
	if err != nil {
		return fmt.Errorf("delete channel record: %w", err)
	}
	if count == 0 {
		return ErrInvalidChannel
	}
	return nil
}
