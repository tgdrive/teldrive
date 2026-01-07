package services

import (
	"context"
	"net/http"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/td/telegram"
	"github.com/ogen-go/ogen/ogenerrors"
	"go.uber.org/zap"

	ht "github.com/ogen-go/ogen/http"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/events"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/internal/version"
	"github.com/tgdrive/teldrive/pkg/models"
	"gorm.io/gorm"
)

type apiService struct {
	db             *gorm.DB
	cnf            *config.ServerCmdConfig
	cache          cache.Cacher
	worker         *tgc.BotWorker
	middlewares    []telegram.Middleware
	events         *events.Recorder
	channelManager *tgc.ChannelManager
}

func (a *apiService) VersionVersion(ctx context.Context) (*api.ApiVersion, error) {
	return version.GetVersionInfo(), nil
}

func (a *apiService) EventsGetEvents(ctx context.Context) ([]api.Event, error) {
	//Get latest events within 5 minutes
	userId := auth.GetUser(ctx)
	res := []models.Event{}
	a.db.Model(&models.Event{}).Where("created_at > ?", time.Now().UTC().Add(-10*time.Minute).Format(time.RFC3339)).
		Where("user_id = ?", userId).Order("created_at desc").Find(&res)
	return utils.Map(res, func(item models.Event) api.Event {
		return api.Event{
			ID:        item.ID,
			Type:      item.Type,
			CreatedAt: item.CreatedAt,
			Source: api.Source{
				ID:           item.Source.Data().ID,
				Type:         api.SourceType(item.Source.Data().Type),
				Name:         item.Source.Data().Name,
				ParentId:     item.Source.Data().ParentID,
				DestParentId: api.NewOptString(item.Source.Data().DestParentID),
			},
		}
	}), nil
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
		if apiError.code == 0 {
			code = http.StatusInternalServerError
			message = http.StatusText(code)
		} else {
			code = apiError.code
			message = apiError.Error()
		}
		logging.FromContext(ctx).Error("api error", zap.Error(apiError.err))
	}
	return &api.ErrorStatusCode{StatusCode: code, Response: api.Error{Code: code, Message: message}}
}

func NewApiService(db *gorm.DB,
	cnf *config.ServerCmdConfig,
	cache cache.Cacher,
	worker *tgc.BotWorker,
	events *events.Recorder) *apiService {

	middlewares := tgc.NewMiddleware(&cnf.TG, tgc.WithFloodWait(), tgc.WithRateLimit(), tgc.WithRetry(5))

	return &apiService{
		db:             db,
		cnf:            cnf,
		cache:          cache,
		worker:         worker,
		middlewares:    middlewares,
		events:         events,
		channelManager: tgc.NewChannelManager(db, cache, &cnf.TG, middlewares),
	}
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

func (a *apiError) Unwrap() error {
	return a.err
}

var (
	_ api.Handler = (*apiService)(nil)
	_ error       = apiError{}
)
