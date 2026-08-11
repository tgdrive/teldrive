//go:build integration

package health_test

import (
	"context"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/health"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestReadyAgainstRealPostgres(t *testing.T) {
	db := testpostgres.New(t)
	status, err := health.NewService("integration", db.Pool).Ready(context.Background())
	if err != nil {
		t.Fatalf("Ready() error = %v", err)
	}
	if status.State != "ok" || status.Version != "integration" {
		t.Fatalf("Ready() = %#v", status)
	}
}
