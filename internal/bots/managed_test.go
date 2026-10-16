package bots

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestManagedInputValidation(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"hello_bot", " @HelloBot ", strings.Repeat("a", 29) + "bot"} {
		if _, err := NormalizeBotUsername(value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []string{"bot", "abcd", "my-bot", "bot!", strings.Repeat("a", 30) + "bot", "ñame_bot"} {
		if _, err := NormalizeBotUsername(value); !errors.Is(err, ErrBotUsernameInvalid) {
			t.Fatalf("accepted invalid username: %q", value)
		}
	}
	service := &Service{}
	if _, _, err := service.CreateManaged(context.Background(), 1, 2, " ", "storage_bot"); !errors.Is(err, ErrInvalidInput) {
		t.Fatal(err)
	}
	if _, _, err := service.CreateManaged(context.Background(), 1, 2, "Storage", "storage_bot"); !errors.Is(err, ErrManagedUnavailable) {
		t.Fatal(err)
	}
}
