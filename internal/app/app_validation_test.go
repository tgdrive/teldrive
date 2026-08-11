package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tgdrive/teldrive/v2/internal/app"
	"github.com/tgdrive/teldrive/v2/internal/config"
)

func TestNewRequiresSecurityDataKeyBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.Security.DataKey = ""

	_, err := app.New(context.Background(), cfg, app.Dependencies{})
	if err == nil || !strings.Contains(err.Error(), "TELDRIVE_SECURITY_DATA_KEY") {
		t.Fatalf("New() error = %v", err)
	}
}
