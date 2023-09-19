package kv

import (
	"encoding/json"
)

func GetValue(kv KV, key string, target interface{}) error {
	data, err := kv.Get(key)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, target); err != nil {
		return err
	}

	return nil
}

func SetValue(kv KV, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return kv.Set(key, data)
}
