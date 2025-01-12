package services

import (
	"context"
	"net/http"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram"
	"github.com/ogen-go/ogen/ogenerrors"
	"go.etcd.io/bbolt"

	ht "github.com/ogen-go/ogen/http"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/version"
	"gorm.io/gorm"
)

type apiService struct {
	db          *gorm.DB
	cnf         *config.ServerCmdConfig
	cache       cache.Cacher
	boltdb      *bbolt.DB
	worker      *tgc.BotWorker
	middlewares []telegram.Middleware
}

func (a *apiService) VersionVersion(ctx context.Context) (*api.ApiVersion, error) {
	return version.GetVersionInfo(), nil
}

func (a *apiService) NewError(ctx context.Context, err error) *api.ErrorStatusCode {
	var (
		code     = http.StatusInternalServerError
		message  = http.StatusText(code)
		ogenErr  ogenerrors.Error
		apiError *apiError
	)
	switch {
	case errors.Is(err, ht.ErrNotImplemented):
		code = http.StatusNotImplemented
		message = http.StatusText(code)
	case errors.As(err, &ogenErr):
		code = ogenErr.Code()
		message = ogenErr.Error()
	case errors.As(err, &apiError):
		code = apiError.Code()
		message = apiError.Error()
	}
	return &api.ErrorStatusCode{StatusCode: code, Response: api.Error{Code: code, Message: message}}
}

func NewApiService(db *gorm.DB,
	cnf *config.ServerCmdConfig,
	cache cache.Cacher,
	boltdb *bbolt.DB,
	worker *tgc.BotWorker) *apiService {
	return &apiService{db: db, cnf: cnf, cache: cache, boltdb: boltdb, worker: worker,
		middlewares: tgc.NewMiddleware(&cnf.TG, tgc.WithFloodWait(), tgc.WithRateLimit())}
}

type extendedService struct {
	api *apiService
}

func NewExtendedService(api *apiService) *extendedService {
	return &extendedService{api: api}
}

type extendedMiddleware struct {
	next *api.Server
	srv  *extendedService
}

func (m *extendedMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route, ok := m.next.FindRoute(r.Method, r.URL.Path)
	if !ok {
		m.next.ServeHTTP(w, r)
		return
	}
	switch route.Name() {
	case api.AuthWsOperation:
		m.srv.AuthWs(w, r)
		return
	case api.FilesStreamOperation:
		args := route.Args()
		m.srv.FilesStream(w, r, args[0], 0)
		return
	case api.SharesStreamOperation:
		args := route.Args()
		m.srv.SharesStream(w, r, args[0], args[1])
		return
	}
	m.next.ServeHTTP(w, r)
}

func NewExtendedMiddleware(next *api.Server, srv *extendedService) *extendedMiddleware {
	return &extendedMiddleware{next: next, srv: srv}
}

type apiError struct {
	err  error
	code int
}

func (a apiError) Error() string {
	return a.err.Error()
}

func (a *apiError) Code() int {
	if a.code == 0 {
		return http.StatusInternalServerError
	}
	return a.code
}

func (a *apiError) Unwrap() error {
	return a.err
}

var (
	_ api.Handler = (*apiService)(nil)
	_ error       = apiError{}
)
