package main

import "testing"

func TestResolveVersion(t *testing.T) {
	for _, tc := range []struct{ input, tags, want string }{
		{"patch", "1.8.3 v1.9.0 v2.0.0-rc.1", "1.9.1"},
		{"minor", "1.8.3", "1.9.0"},
		{"major", "1.8.3", "2.0.0"},
		{"current", "1.8.3 v1.10.0 1.9.9", "1.10.0"},
		{"v2.0.0", "", "2.0.0"},
	} {
		got, err := resolveVersion(tc.input, tc.tags)
		if err != nil || got.String() != tc.want {
			t.Fatalf("resolveVersion(%q) = %v, %v; want %s", tc.input, got, err, tc.want)
		}
	}
	for _, input := range []string{"2.01.0", "2.0", "2.0.0-rc.1", "bad", "patch"} {
		if _, err := resolveVersion(input, ""); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}
