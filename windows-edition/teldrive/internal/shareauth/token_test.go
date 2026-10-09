package shareauth

import (
	"testing"
	"time"
)

func TestBrowserGrant(t *testing.T) {
	now := time.Unix(1000, 0)
	token := Issue("share-a", "hashed-password", now.Add(time.Hour))
	if !Valid(token, "share-a", "hashed-password", now) {
		t.Fatal("valid grant rejected")
	}
	cases := []struct {
		name, token, id, secret string
		now                     time.Time
	}{
		{"different share", token, "share-b", "hashed-password", now},
		{"password changed", token, "share-a", "new-hash", now},
		{"expired", token, "share-a", "hashed-password", now.Add(time.Hour)},
		{"tampered", "999999." + token, "share-a", "hashed-password", now},
		{"malformed", "broken", "share-a", "hashed-password", now},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if Valid(tc.token, tc.id, tc.secret, tc.now) {
				t.Fatal("invalid grant accepted")
			}
		})
	}
}
