package api

import (
	"encoding/json"
	"testing"
)

func TestRedactJobArgs(t *testing.T) {
	input := map[string]json.RawMessage{
		"user_id": json.RawMessage(`1001`),
		"headers": json.RawMessage(`{"Authorization":"Bearer secret","X-Custom":"private"}`),
		"sources": json.RawMessage(`[{"url":"https://example.test/file","headers":{"Cookie":"session=secret"},"password":"hidden"}]`),
	}

	redacted := redactJobArgs(input)
	if string(redacted["user_id"]) != "1001" {
		t.Fatalf("user_id = %s, want 1001", redacted["user_id"])
	}

	var headers map[string]string
	if err := json.Unmarshal(redacted["headers"], &headers); err != nil {
		t.Fatal(err)
	}
	for key, value := range headers {
		if value != "[redacted]" {
			t.Fatalf("header %s = %q, want redacted", key, value)
		}
	}

	var sources []map[string]any
	if err := json.Unmarshal(redacted["sources"], &sources); err != nil {
		t.Fatal(err)
	}
	if got := sources[0]["url"]; got != "https://example.test/file" {
		t.Fatalf("url = %#v", got)
	}
	if got := sources[0]["password"]; got != "[redacted]" {
		t.Fatalf("password = %#v", got)
	}
	nested := sources[0]["headers"].(map[string]any)
	if got := nested["Cookie"]; got != "[redacted]" {
		t.Fatalf("cookie = %#v", got)
	}
}
