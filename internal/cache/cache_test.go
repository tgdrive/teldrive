package cache

import (
	"context"
	"testing"
	"time"

	"github.com/divyam234/teldrive/pkg/schemas"
	"github.com/stretchr/testify/assert"
)

func TestCache(t *testing.T) {
	ctx := context.Background()
	cache := FromContext(ctx)

	var value = schemas.FileIn{
		Name: "file.jpeg",
		Type: "file",
	}
	var result schemas.FileIn

	err := cache.Set("key", value, 1*time.Minute)
	assert.NoError(t, err)

	err = cache.Get("key", &result)
	assert.NoError(t, err)
	assert.Equal(t, result, value)
}
