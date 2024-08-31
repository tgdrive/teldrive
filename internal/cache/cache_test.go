package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tgdrive/teldrive/pkg/schemas"
)

func TestCache(t *testing.T) {

	var value = schemas.FileIn{
		Name: "file.jpeg",
		Type: "file",
	}
	var result schemas.FileIn

	cache := NewMemoryCache(1 * 1024 * 1024)

	err := cache.Set("key", value, 1*time.Second)
	assert.NoError(t, err)

	err = cache.Get("key", &result)
	assert.NoError(t, err)
	assert.Equal(t, result, value)
}
