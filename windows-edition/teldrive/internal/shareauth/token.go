// Package shareauth signs short-lived browser grants for password-protected media.
package shareauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

func Issue(shareID, secret string, expires time.Time) string {
	payload := strconv.FormatInt(expires.Unix(), 10)
	return payload + "." + signature(shareID, secret, payload)
}

func Valid(token, shareID, secret string, now time.Time) bool {
	payload, sig, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	expires, err := strconv.ParseInt(payload, 10, 64)
	if err != nil || now.Unix() >= expires {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(signature(shareID, secret, payload)))
}

func signature(shareID, secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(shareID + ":" + payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
