package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tgdrive/teldrive/pkg/models"
)

func TestCache(t *testing.T) {

	var value = models.File{
		Name: "file.jpeg",
		Type: "file",
	}
	var result models.File

	cache := NewMemoryCache(1 * 1024 * 1024)

	err := cache.Set("key", value, 1*time.Second)
	assert.NoError(t, err)

	err = cache.Get("key", &result)
	assert.NoError(t, err)
	assert.Equal(t, result, value)
}

func TestKey(t *testing.T) {
	tests := []struct {
		name     string
		args     []interface{}
		expected string
	}{
		{
			name:     "simple strings",
			args:     []interface{}{"user", "123"},
			expected: "user:123",
		},
		{
			name:     "mixed types",
			args:     []interface{}{"cache", 123, true},
			expected: "cache:123:true",
		},
		{
			name:     "with nil",
			args:     []interface{}{"key", nil, "value"},
			expected: "key:nil:value",
		},
		{
			name:     "empty args",
			args:     []interface{}{},
			expected: "",
		},
		{
			name:     "single arg",
			args:     []interface{}{"solo"},
			expected: "solo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Key(tt.args...)
			if result != tt.expected {
				t.Errorf("Key() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFormatValue(t *testing.T) {
	type testStruct struct {
		Name string
		Age  int
	}

	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "nil value",
			input:    nil,
			expected: "nil",
		},
		{
			name:     "string",
			input:    "test",
			expected: "test",
		},
		{
			name:     "integer",
			input:    123,
			expected: "123",
		},
		{
			name:     "boolean",
			input:    true,
			expected: "true",
		},
		{
			name:     "slice of strings",
			input:    []string{"a", "b", "c"},
			expected: "[a,b,c]",
		},
		{
			name:     "slice of ints",
			input:    []int{1, 2, 3},
			expected: "[1,2,3]",
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: "[]",
		},
		{
			name:     "map string to string",
			input:    map[string]string{"a": "1", "b": "2"},
			expected: "{a=1,b=2}",
		},
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: "{}",
		},
		{
			name:     "struct",
			input:    testStruct{Name: "John", Age: 30},
			expected: "{Name:John Age:30}",
		},
		{
			name:     "pointer to string",
			input:    func() interface{} { s := "test"; return &s }(),
			expected: "test",
		},
		{
			name:     "nil pointer",
			input:    func() interface{} { var s *string; return s }(),
			expected: "nil",
		},
		{
			name:     "nested slice",
			input:    [][]int{{1, 2}, {3, 4}},
			expected: "[[1,2],[3,4]]",
		},
		{
			name: "complex mixed structure",
			input: struct {
				ID    int
				Tags  []string
				Meta  map[string]interface{}
				Valid bool
			}{
				ID:    1,
				Tags:  []string{"a", "b"},
				Meta:  map[string]interface{}{"count": 42},
				Valid: true,
			},
			expected: "{ID:1 Tags:[a b] Meta:map[count:42] Valid:true}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatValue(tt.input)
			if result != tt.expected {
				t.Errorf("formatValue() = %v, want %v", result, tt.expected)
			}
		})
	}
}
