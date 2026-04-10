package services

import (
	"context"
	"net/http"
	"time"

	"github.com/go-faster/errors"
	"github.com/ogen-go/ogen/ogenerrors"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/zap"

	ht "github.com/ogen-go/ogen/http"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/events"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/internal/version"
	"github.com/tgdrive/teldrive/pkg/mapper"
	"github.com/tgdrive/teldrive/pkg/repositories"
)

type apiService struct {
	cnf            *config.ServerCmdConfig
	cache          cache.Cacher
	events         events.EventBroadcaster
	authAttempts   *authAttemptManager
	channelManager ChannelManager
	telegram       TelegramService
	repo           *repositories.Repositories
	jobs           jobClient
	periodicJobs   periodicJobRegistry
}

type periodicJobRegistry interface {
	AddMany(periodicJobs []*river.PeriodicJob) []rivertype.PeriodicJobHandle
	AddSafely(periodicJob *river.PeriodicJob) (rivertype.PeriodicJobHandle, error)
	RemoveByID(id string) bool
}

func (a *apiService) VersionVersion(ctx context.Context) (*api.ApiVersion, error) {
	return version.VersionInfo(), nil
}

func (a *apiService) EventsGetEvents(ctx context.Context) ([]api.Event, error) {
	//Get latest events within 5 minutes
	userId := auth.User(ctx)
	res, err := a.repo.Events.GetRecent(ctx, userId, time.Now().UTC().Add(-10*time.Minute), 100)
	if err != nil {
		return nil, &apiError{err: err}
	}
	return utils.Map(res, mapper.ToEventOut), nil
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
		logger := logging.Component("API")
		logger.Error("request.failed", zap.Error(apiError.err))
	}
	return &api.ErrorStatusCode{StatusCode: code, Response: api.Error{Code: code, Message: message}}
}

func NewApiService(repo *repositories.Repositories,
	channelManager ChannelManager,
	cnf *config.ServerCmdConfig,
	cache cache.Cacher,
	telegram TelegramService,
	events events.EventBroadcaster,
	jobs jobClient,
	periodicJobs periodicJobRegistry) *apiService {

	return &apiService{
		repo:           repo,
		cnf:            cnf,
		cache:          cache,
		events:         events,
		authAttempts:   newAuthAttemptManager(),
		channelManager: channelManager,
		telegram:       telegram,
		jobs:           jobs,
		periodicJobs:   periodicJobs,
	}
}

func (a *apiService) syncRunMaxAttempts() int {
	if a == nil || a.cnf == nil || a.cnf.Jobs.SyncRun.MaxAttempts <= 0 {
		return 8
	}
	return a.cnf.Jobs.SyncRun.MaxAttempts
}

func (a *apiService) syncTransferMaxAttempts() int {
	if a == nil || a.cnf == nil || a.cnf.Jobs.SyncTransfer.MaxAttempts <= 0 {
		return 2
	}
	return a.cnf.Jobs.SyncTransfer.MaxAttempts
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
