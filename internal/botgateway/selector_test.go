package botgateway

import (
	"context"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

func TestMemoryBotSelectorIsPerUserAndOperation(t *testing.T) {
	selector := new(memoryBotSelector)
	ctx := context.Background()

	assertNext := func(userID int64, operation telegramstore.Operation, want uint64) {
		t.Helper()
		got, err := selector.Next(ctx, userID, operation)
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		if got != want {
			t.Fatalf("Next(%d, %s) = %d, want %d", userID, operation, got, want)
		}
	}

	assertNext(1, telegramstore.OperationUpload, 0)
	assertNext(1, telegramstore.OperationUpload, 1)
	assertNext(2, telegramstore.OperationUpload, 0)
	assertNext(1, telegramstore.OperationDownload, 0)
	assertNext(1, telegramstore.OperationUpload, 2)
}

func TestMemoryBotSelectorRejectsInvalidUser(t *testing.T) {
	selector := new(memoryBotSelector)
	if _, err := selector.Next(context.Background(), 0, telegramstore.OperationUpload); err == nil {
		t.Fatal("Next() accepted invalid user")
	}
}
