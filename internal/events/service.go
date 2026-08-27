package events

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
)

var (
	ErrInvalidCursor = errors.New("invalid event stream cursor")
	ErrInvalidTicket = errors.New("invalid or expired event stream ticket")
)

type Config struct {
	BatchSize             int32
	MaxConnectionsPerUser int
	Heartbeat             time.Duration
	WriteTimeout          time.Duration
	TicketTTL             time.Duration
	Retention             time.Duration
	CleanupInterval       time.Duration
	ConnectTimeout        time.Duration
	PingInterval          time.Duration
	ReconnectMin          time.Duration
	ReconnectMax          time.Duration
}

type Event struct {
	ID           int64
	UserID       int64
	Type         string
	ResourceType string
	ResourceID   string
	Generation   *int64
	Payload      []byte
	OccurredAt   time.Time
}

type Ticket struct {
	Value     string
	ExpiresAt time.Time
}

type Service struct {
	queries  *sqlcgen.Queries
	hub      *Hub
	listener *Listener
	logger   *slog.Logger
	config   Config

	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	running bool
	closed  bool
}

func NewService(pool *pgxpool.Pool, logger *slog.Logger, cfg Config) (*Service, error) {
	if pool == nil {
		return nil, errors.New("event service requires a database pool")
	}
	if logger == nil {
		logger = slog.Default()
	}
	cfg = withDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	hub := NewHub(cfg.MaxConnectionsPerUser)
	done := make(chan struct{})
	close(done)
	return &Service{
		queries: sqlcgen.New(pool),
		hub:     hub,
		listener: newListener(pool, hub, logger, listenerConfig{
			ConnectTimeout: cfg.ConnectTimeout,
			PingInterval:   cfg.PingInterval,
			ReconnectMin:   cfg.ReconnectMin,
			ReconnectMax:   cfg.ReconnectMax,
		}),
		logger: logger,
		config: cfg,
		done:   done,
	}, nil
}

func withDefaults(cfg Config) Config {
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 100
	}
	if cfg.MaxConnectionsPerUser == 0 {
		cfg.MaxConnectionsPerUser = 5
	}
	if cfg.Heartbeat == 0 {
		cfg.Heartbeat = 20 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 10 * time.Second
	}
	if cfg.TicketTTL == 0 {
		cfg.TicketTTL = 2 * time.Minute
	}
	if cfg.Retention == 0 {
		cfg.Retention = 7 * 24 * time.Hour
	}
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = time.Hour
	}
	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = 10 * time.Second
	}
	if cfg.PingInterval == 0 {
		cfg.PingInterval = 5 * time.Second
	}
	if cfg.ReconnectMin == 0 {
		cfg.ReconnectMin = 100 * time.Millisecond
	}
	if cfg.ReconnectMax == 0 {
		cfg.ReconnectMax = 30 * time.Second
	}
	return cfg
}

func validateConfig(cfg Config) error {
	switch {
	case cfg.BatchSize < 1 || cfg.BatchSize > 1000:
		return errors.New("event batch size must be between 1 and 1000")
	case cfg.MaxConnectionsPerUser < 1 || cfg.MaxConnectionsPerUser > 1000:
		return errors.New("event connections per user must be between 1 and 1000")
	case cfg.Heartbeat <= 0:
		return errors.New("event heartbeat must be positive")
	case cfg.WriteTimeout <= 0:
		return errors.New("event write timeout must be positive")
	case cfg.TicketTTL <= 0:
		return errors.New("event ticket TTL must be positive")
	case cfg.Retention <= 0:
		return errors.New("event retention must be positive")
	case cfg.CleanupInterval <= 0:
		return errors.New("event cleanup interval must be positive")
	case cfg.ConnectTimeout <= 0:
		return errors.New("event listener connect timeout must be positive")
	case cfg.PingInterval <= 0:
		return errors.New("event listener ping interval must be positive")
	case cfg.ReconnectMin <= 0 || cfg.ReconnectMax < cfg.ReconnectMin:
		return errors.New("event listener reconnect bounds are invalid")
	default:
		return nil
	}
}

func (s *Service) Start(ctx context.Context) error {
	if s == nil || s.listener == nil {
		return errors.New("event service is not configured")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrServiceClosed
	}
	if s.running {
		return nil
	}

	serviceCtx, cancel := context.WithCancel(ctx)
	if err := s.listener.Start(serviceCtx); err != nil {
		cancel()
		return err
	}
	s.cancel = cancel
	s.running = true
	s.done = make(chan struct{})
	go s.runCleanup(serviceCtx, s.done)
	return nil
}

func (s *Service) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	if !s.running {
		s.mu.Unlock()
		s.hub.Close()
		return nil
	}
	cancel, done := s.cancel, s.done
	s.mu.Unlock()

	cancel()
	s.hub.Close()
	listenerErr := s.listener.Close(ctx)
	select {
	case <-done:
		return listenerErr
	case <-ctx.Done():
		return errors.Join(listenerErr, ctx.Err())
	}
}

func (s *Service) Done() <-chan struct{} {
	if s == nil {
		done := make(chan struct{})
		close(done)
		return done
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.done
}

func (s *Service) Subscribe(userID int64) (<-chan struct{}, func(), error) {
	if s == nil || s.hub == nil {
		return nil, nil, ErrServiceClosed
	}
	return s.hub.Subscribe(userID)
}

func (s *Service) BatchSize() int32 {
	if s == nil {
		return 100
	}
	return s.config.BatchSize
}

func (s *Service) Heartbeat() time.Duration {
	if s == nil {
		return 20 * time.Second
	}
	return s.config.Heartbeat
}

func (s *Service) WriteTimeout() time.Duration {
	if s == nil {
		return 10 * time.Second
	}
	return s.config.WriteTimeout
}

func (s *Service) ListAfter(ctx context.Context, userID, afterID int64, eventTypes []string) ([]Event, error) {
	if s == nil || s.queries == nil || userID <= 0 || afterID < 0 {
		return nil, errors.New("invalid event list request")
	}
	rows, err := s.queries.ListUserEventsAfter(ctx, sqlcgen.ListUserEventsAfterParams{
		UserID: userID, AfterID: afterID, EventTypes: eventTypes, EventLimit: s.config.BatchSize,
	})
	if err != nil {
		return nil, fmt.Errorf("list user events: %w", err)
	}
	events := make([]Event, 0, len(rows))
	for _, row := range rows {
		event, err := eventFromRow(row)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

// CurrentCursor returns the newest event ID known for the user.
func (s *Service) CurrentCursor(ctx context.Context, userID int64) (int64, error) {
	if s == nil || s.queries == nil || userID <= 0 {
		return 0, errors.New("invalid event cursor request")
	}
	state, err := s.queries.GetUserEventCursorState(ctx, sqlcgen.GetUserEventCursorStateParams{
		CursorUserID: userID,
		AfterID:      0,
	})
	if err != nil {
		return 0, fmt.Errorf("get current event cursor: %w", err)
	}
	return state.LastEventID, nil
}

func (s *Service) CursorExpired(ctx context.Context, userID, afterID int64) (bool, error) {
	if afterID <= 0 {
		return false, nil
	}
	state, err := s.queries.GetUserEventCursorState(ctx, sqlcgen.GetUserEventCursorStateParams{
		CursorUserID: userID,
		AfterID:      afterID,
	})
	if err != nil {
		return false, fmt.Errorf("get event cursor state: %w", err)
	}
	if afterID > state.LastEventID {
		return false, ErrInvalidCursor
	}
	return !state.CursorExists && state.LastEventID > afterID, nil
}

func (s *Service) IssueTicket(ctx context.Context, userID int64) (Ticket, error) {
	if s == nil || s.queries == nil || userID <= 0 {
		return Ticket{}, errors.New("invalid event ticket request")
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return Ticket{}, ErrServiceClosed
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Ticket{}, fmt.Errorf("generate event stream ticket: %w", err)
	}
	value := base64.RawURLEncoding.EncodeToString(raw)
	expiresAt := time.Now().UTC().Add(s.config.TicketTTL)
	hash := sha256.Sum256([]byte(value))
	if err := s.queries.CreateEventStreamTicket(ctx, sqlcgen.CreateEventStreamTicketParams{
		TokenHash: hash[:],
		UserID:    userID,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}); err != nil {
		return Ticket{}, fmt.Errorf("store event stream ticket: %w", err)
	}
	return Ticket{Value: value, ExpiresAt: expiresAt}, nil
}

func (s *Service) AuthenticateTicket(ctx context.Context, value string) (int64, error) {
	if s == nil || s.queries == nil || strings.TrimSpace(value) == "" {
		return 0, ErrInvalidTicket
	}
	hash := sha256.Sum256([]byte(value))
	userID, err := s.queries.GetEventStreamTicketUser(ctx, hash[:])
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrInvalidTicket
	}
	if err != nil {
		return 0, fmt.Errorf("authenticate event stream ticket: %w", err)
	}
	if userID <= 0 {
		return 0, ErrInvalidTicket
	}
	return userID, nil
}

func (s *Service) runCleanup(ctx context.Context, done chan struct{}) {
	defer func() {
		s.mu.Lock()
		s.running = false
		s.cancel = nil
		close(done)
		s.mu.Unlock()
	}()

	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanupCtx, cancel := context.WithTimeout(ctx, s.config.ConnectTimeout)
			if _, err := s.queries.DeleteUserEventsBefore(cleanupCtx, pgtype.Timestamptz{
				Time: time.Now().UTC().Add(-s.config.Retention), Valid: true,
			}); err != nil && !errors.Is(err, context.Canceled) {
				s.logger.ErrorContext(ctx, "delete expired user events", "error", err)
			}
			if _, err := s.queries.DeleteExpiredEventStreamTickets(cleanupCtx); err != nil && !errors.Is(err, context.Canceled) {
				s.logger.ErrorContext(ctx, "delete expired event stream tickets", "error", err)
			}
			cancel()
		}
	}
}

func eventFromRow(row *sqlcgen.UserEvent) (Event, error) {
	if row == nil || row.ID <= 0 || row.UserID <= 0 || !row.OccurredAt.Valid {
		return Event{}, errors.New("invalid stored user event")
	}
	event := Event{
		ID:           row.ID,
		UserID:       row.UserID,
		Type:         row.EventType,
		ResourceType: row.ResourceType,
		Payload:      append([]byte(nil), row.Payload...),
		OccurredAt:   row.OccurredAt.Time.UTC(),
	}
	if row.ResourceID.Valid {
		event.ResourceID = row.ResourceID.String
	}
	if row.Generation.Valid {
		generation := row.Generation.Int64
		event.Generation = &generation
	}
	return event, nil
}
