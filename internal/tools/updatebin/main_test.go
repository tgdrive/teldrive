package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLatestVersion(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
		want   string
	}{
		{`{"tag_name":"2.0.0","draft":false,"prerelease":false}`, 200, "2.0.0"},
		{`{"tag_name":"v2.0.0","draft":true}`, 200, ""},
		{`{"tag_name":"v2.0.0-rc.1","prerelease":true}`, 200, ""},
		{`{}`, 200, ""},
		{`{}`, 404, ""},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); w.Write([]byte(tc.body)) }))
		got, err := latestVersion(server.Client(), server.URL)
		server.Close()
		if got != tc.want || (err != nil) != (tc.want == "") {
			t.Fatalf("latestVersion()=%q,%v; want %q", got, err, tc.want)
		}
	}
}
