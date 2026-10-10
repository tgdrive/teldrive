package botgateway

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

const (
	RotationMemory   = "memory"
	RotationDatabase = "database"
)

type botSelector interface {
	Next(context.Context, int64, telegramstore.Operation) (uint64, error)
}

type memoryBotSelector struct {
	counters sync.Map
}

type memoryCounterKey struct {
	userID    int64
	operation telegramstore.Operation
}

func (s *memoryBotSelector) Next(_ context.Context, userID int64, operation telegramstore.Operation) (uint64, error) {
	if userID <= 0 {
		return 0, errors.New("invalid bot selection user")
	}
	value, _ := s.counters.LoadOrStore(memoryCounterKey{userID: userID, operation: operation}, new(atomic.Uint64))
	return value.(*atomic.Uint64).Add(1) - 1, nil
}

type databaseBotSelector struct {
	queries *sqlcgen.Queries
}

func (s databaseBotSelector) Next(ctx context.Context, userID int64, operation telegramstore.Operation) (uint64, error) {
	value, err := s.queries.NextBotSelectionValue(ctx, sqlcgen.NextBotSelectionValueParams{
		UserID: userID, Operation: string(operation),
	})
	return uint64(value), err
}
