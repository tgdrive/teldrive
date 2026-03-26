package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ogen-go/ogen/ogenerrors"
	"github.com/tgdrive/teldrive/internal/api"
	authpkg "github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/database"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/requestmeta"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/pkg/repositories"
	"github.com/tgdrive/teldrive/pkg/services"
	"github.com/tgdrive/teldrive/pkg/types"
	"go.uber.org/zap/zapcore"
)

func init() {
	logging.SetLevel(zapcore.FatalLevel)
}

type suite struct {
	t       *testing.T
	ctx     context.Context
	cfg     *config.ServerCmdConfig
	repos   *repositories.Repositories
	server  *httptest.Server
	pool    *pgxpool.Pool
	cache   cache.Cacher
	tgMock  *mockTelegramService
	events  *noopEventBroadcaster
	httpCli *http.Client
}

type testSecuritySource struct {
	bearer  string
	cookie  string
	xAPIKey string
	shash   string
}

func (s testSecuritySource) AccessTokenCookieAuth(context.Context, api.OperationName) (api.AccessTokenCookieAuth, error) {
	if s.cookie == "" {
		return api.AccessTokenCookieAuth{}, ogenerrors.ErrSkipClientSecurity
	}
	return api.AccessTokenCookieAuth{APIKey: s.cookie}, nil
}

func (s testSecuritySource) XApiKeyHeaderAuth(context.Context, api.OperationName) (api.XApiKeyHeaderAuth, error) {
	if s.xAPIKey == "" {
		return api.XApiKeyHeaderAuth{}, ogenerrors.ErrSkipClientSecurity
	}
	return api.XApiKeyHeaderAuth{APIKey: s.xAPIKey}, nil
}

func (s testSecuritySource) BearerAuth(context.Context, api.OperationName) (api.BearerAuth, error) {
	if s.bearer == "" {
		return api.BearerAuth{}, ogenerrors.ErrSkipClientSecurity
	}
	return api.BearerAuth{Token: s.bearer}, nil
}

func (s testSecuritySource) SessionHashAuth(context.Context, api.OperationName) (api.SessionHashAuth, error) {
	if s.shash == "" {
		return api.SessionHashAuth{}, ogenerrors.ErrSkipClientSecurity
	}
	return api.SessionHashAuth{APIKey: s.shash}, nil
}

func newSuite(t *testing.T) *suite {
	t.Helper()

	if os.Getenv("TEST_DATABASE_URL") == "" && os.Getenv("DATABASE_URL") == "" {
		t.Skip("set TEST_DATABASE_URL (or DATABASE_URL). Example: postgres://teldrive_test:teldrive_test@localhost:55432/teldrive_test?sslmode=disable")
	}

	ctx := context.Background()
	pool := database.NewTestDatabase(t, true)

	t.Cleanup(func() {
		pool.Close()
	})

	cfg := &config.ServerCmdConfig{}
	cfg.JWT.Secret = "test-secret"
	cfg.JWT.SessionTime = 24 * time.Hour
	cfg.TG.PoolSize = 1
	cfg.TG.Uploads.MaxRetries = 1
	cfg.TG.Uploads.Threads = 1
	cfg.TG.Uploads.Retention = time.Hour

	repos := repositories.NewRepositories(pool)
	tgMock := newMockTelegramService()
	evt := newNoopEventBroadcaster()
	c := cache.NewMemoryCache(10 * 1024 * 1024)
	channelManager := tgc.NewChannelManager(repos, c, &cfg.TG)

	h := services.NewApiService(repos, channelManager, cfg, c, tgMock, evt, newNoopJobClient())
	sec := authpkg.NewSecurityHandler(repos.Sessions, repos.APIKeys, c, &cfg.JWT)
	rawSrv := services.NewRawService(h)
	srv, err := api.NewServer(h, rawSrv, sec)
	if err != nil {
		t.Fatalf("create API server: %v", err)
	}
	httpSrv := httptest.NewServer(requestmeta.Middleware(srv))
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}

	t.Cleanup(httpSrv.Close)

	s := &suite{
		t:       t,
		ctx:     ctx,
		cfg:     cfg,
		repos:   repos,
		server:  httpSrv,
		pool:    pool,
		cache:   c,
		tgMock:  tgMock,
		events:  evt,
		httpCli: &http.Client{Transport: httpSrv.Client().Transport, Jar: jar},
	}

	s.resetDB()

	return s
}

func (s *suite) resetDB() {
	s.t.Helper()

	_, err := s.pool.Exec(s.ctx, "TRUNCATE TABLE teldrive.events, teldrive.file_shares, teldrive.uploads, teldrive.files, teldrive.sessions, teldrive.bots, teldrive.channels, teldrive.users, teldrive.kv, teldrive.periodic_jobs RESTART IDENTITY CASCADE")
	if err != nil {
		s.t.Fatalf("truncate test tables: %v", err)
	}
}

func (s *suite) authTokenForUser(userID int64, tgSession string) string {
	s.t.Helper()

	s.ensureUserExists(userID)

	sessionID := deterministicSessionID(userID, tgSession)
	sessionUUID := uuid.MustParse(sessionID)
	if _, err := s.repos.Sessions.GetByID(s.ctx, sessionUUID); err != nil {
		now := time.Now().UTC()
		if err := s.repos.Sessions.Create(s.ctx, &jetmodel.Sessions{ID: sessionUUID, UserID: userID, TgSession: tgSession, CreatedAt: now, UpdatedAt: now}); err != nil {
			s.t.Fatalf("create test session: %v", err)
		}
	}

	now := time.Now().UTC()
	claims := &types.JWTClaims{
		Name:      "Test User",
		UserName:  "test_user",
		IsPremium: false,
		SessionID: sessionUUID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(2 * time.Hour)),
		},
	}

	token, err := authpkg.Encode(s.cfg.JWT.Secret, claims)
	if err != nil {
		s.t.Fatalf("encode JWT: %v", err)
	}

	return token
}

func (s *suite) newClientWithToken(token string) *api.Client {
	s.t.Helper()

	sec := testSecuritySource{}
	if strings.HasPrefix(token, "tdk_") {
		sec.xAPIKey = token
	} else {
		sec.bearer = token
		sec.cookie = token
	}

	client, err := api.NewClient(s.server.URL, sec, api.WithClient(s.httpCli))
	if err != nil {
		s.t.Fatalf("create ogen client: %v", err)
	}

	return client
}
func deterministicSessionID(userID int64, tgSession string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%d:%s", userID, tgSession))).String()
}

func (s *suite) ensureUserExists(userID int64) {
	s.t.Helper()

	_, err := s.repos.Users.GetByID(s.ctx, userID)
	if err == nil {
		return
	}

	now := time.Now().UTC()
	name := "test-user"
	if createErr := s.repos.Users.Create(s.ctx, &jetmodel.Users{
		UserID:    userID,
		UserName:  fmt.Sprintf("user_%d", userID),
		Name:      &name,
		IsPremium: false,
		CreatedAt: now,
		UpdatedAt: now,
	}); createErr != nil {
		s.t.Fatalf("ensure user exists: %v", createErr)
	}
}
