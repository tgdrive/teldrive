package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type JSONB[T any] struct {
	Data T
}

func NewJSONB[T any](value T) JSONB[T] {
	return JSONB[T]{Data: value}
}

func ToRawJSONB[T any](value T) (JSONB[json.RawMessage], error) {
	b, err := json.Marshal(value)
	if err != nil {
		return JSONB[json.RawMessage]{}, err
	}
	return JSONB[json.RawMessage]{Data: json.RawMessage(b)}, nil
}

func (j *JSONB[T]) Scan(src any) error {
	if src == nil {
		var zero T
		j.Data = zero
		return nil
	}

	var data []byte
	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("jsonb scan: unsupported source type %T", src)
	}

	if len(data) == 0 {
		var zero T
		j.Data = zero
		return nil
	}

	if err := json.Unmarshal(data, &j.Data); err != nil {
		return fmt.Errorf("jsonb scan: %w", err)
	}
	return nil
}

func (j JSONB[T]) ValueDB() (driver.Value, error) {
	b, err := json.Marshal(j.Data)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (j JSONB[T]) Value() (driver.Value, error) {
	return j.ValueDB()
}
