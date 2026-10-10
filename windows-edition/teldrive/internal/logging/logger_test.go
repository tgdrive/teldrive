package logging

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestConsoleTimeFormatSurvivesContextFields(t *testing.T) {
	for _, fixture := range []struct {
		name, layout, prefix string
	}{
		{"default", "", "2026-10-09 17:30:45"},
		{"custom", "02/01/2006 15:04", "09/10/2026 17:30"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			logger := NewLogger(&Config{Level: zapcore.InfoLevel, TimeFormat: fixture.layout})
			core, ok := logger.Core().(*prettyCore)
			if !ok {
				t.Fatal("expected a console core")
			}
			var output bytes.Buffer
			core.out = zapcore.AddSync(&output)
			contextCore := core.With([]zapcore.Field{zap.String("component", "test")})
			err := contextCore.Write(zapcore.Entry{
				Time:    time.Date(2026, time.October, 9, 17, 30, 45, 0, time.UTC),
				Level:   zapcore.InfoLevel,
				Message: "configured timestamp",
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(output.String(), fixture.prefix+"  ") || !strings.Contains(output.String(), "[TEST]") {
				t.Fatalf("unexpected console output: %q", output.String())
			}
		})
	}
}
