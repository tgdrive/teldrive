package utils

import (
	"context"
	"fmt"
	"strings"
	"time"

	"reflect"

	"github.com/gotd/td/tg"
	"golang.org/x/exp/constraints"

	"unicode"
)

func Max[T constraints.Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func CamelToPascalCase(input string) string {
	var result strings.Builder
	upperNext := true

	for _, char := range input {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			if upperNext {
				result.WriteRune(unicode.ToUpper(char))
				upperNext = false
			} else {
				result.WriteRune(char)
			}
		} else {
			upperNext = true
		}
	}

	return result.String()
}

func GetField(v interface{}, field string) string {
	r := reflect.ValueOf(v)
	f := reflect.Indirect(r).FieldByName(field)
	fieldValue := f.Interface()

	switch v := fieldValue.(type) {
	case string:
		return v
	case time.Time:
		return v.Format(time.RFC3339)
	default:
		return ""
	}
}

func GetChannelById(ctx context.Context, client *tg.Client, channelID int64) (*tg.Channel, error) {
	inputChannel := &tg.InputChannel{
		ChannelID:  channelID,
		AccessHash: 0,
	}
	channels, err := client.ChannelsGetChannels(ctx, []tg.InputChannelClass{inputChannel})

	if err != nil {
		return nil, fmt.Errorf("failed to fetch channel: %w", err)
	}

	if len(channels.GetChats()) == 0 {
		return nil, fmt.Errorf("no channels found")
	}

	channel := channels.GetChats()[0].(*tg.Channel)
	return channel, nil
}

func BoolPointer(b bool) *bool {
	return &b
}
