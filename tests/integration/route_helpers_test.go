package integration_test

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/tgdrive/teldrive/internal/api"
	authpkg "github.com/tgdrive/teldrive/internal/auth"
)

func loginWithClient(t *testing.T, s *suite, userID int64, username string) (*api.Client, *api.Client, string) {
	t.Helper()

	public := s.newClientWithToken("")
	ctx := context.Background()
	session := "1BvXNhK1zA5P-FAKE-SESSION-" + strconv.FormatInt(userID, 10)

	loginRes, err := public.AuthLogin(ctx, &api.SessionCreate{
		Session:   session,
		UserId:    userID,
		UserName:  username,
		Name:      username,
		IsPremium: false,
	})
	if err != nil {
		t.Fatalf("AuthLogin failed: %v", err)
	}

	token, err := tokenFromSetCookie(loginRes.SetCookie)
	if err != nil {
		t.Fatalf("parse token from Set-Cookie: %v", err)
	}

	claims, err := authpkg.Decode(s.cfg.JWT.Secret, token)
	if err != nil {
		t.Fatalf("decode token claims failed: %v", err)
	}

	return public, s.newClientWithToken(token), claims.SessionID
}

func loginAndGetToken(t *testing.T, s *suite, userID int64, username string) string {
	t.Helper()

	public := s.newClientWithToken("")
	ctx := context.Background()
	session := "1BvXNhK1zA5P-FAKE-SESSION-" + strconv.FormatInt(userID, 10)

	loginRes, err := public.AuthLogin(ctx, &api.SessionCreate{
		Session:   session,
		UserId:    userID,
		UserName:  username,
		Name:      username,
		IsPremium: false,
	})
	if err != nil {
		t.Fatalf("AuthLogin failed: %v", err)
	}

	token, err := tokenFromSetCookie(loginRes.SetCookie)
	if err != nil {
		t.Fatalf("parse token from Set-Cookie: %v", err)
	}

	return token
}

func tokenFromSetCookie(setCookie string) (string, error) {
	parts := strings.Split(setCookie, ";")
	if len(parts) == 0 {
		return "", errors.New("empty Set-Cookie header")
	}
	kv := strings.SplitN(parts[0], "=", 2)
	if len(kv) != 2 || kv[1] == "" {
		return "", errors.New("access token cookie not found")
	}
	return kv[1], nil
}

func statusCode(err error) int {
	if err == nil {
		return 200
	}
	var statusErr *api.ErrorStatusCode
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode
	}
	re := regexp.MustCompile(`code\s+(\d{3})`)
	m := re.FindStringSubmatch(err.Error())
	if len(m) == 2 {
		if n, convErr := strconv.Atoi(m[1]); convErr == nil {
			return n
		}
	}
	return -1
}
