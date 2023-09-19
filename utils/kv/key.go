package kv

import (
	"strings"
)

func Key(indexes ...string) string {
	return strings.Join(indexes, ":")
}
