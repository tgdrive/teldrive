package services

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
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
	botSelector    tgc.BotSelector
	events         events.EventBroadcaster
	channelManager *tgc.ChannelManager
	clientPool     *tgc.ClientPool
}

func (a *apiService) newMiddlewares(ctx context.Context, retries int) []telegram.Middleware {
	return tgc.NewMiddleware(&a.cnf.TG,
		tgc.WithFloodWait(),
		tgc.WithRecovery(ctx),
		tgc.WithRetry(retries),
		tgc.WithRateLimit(),
	)
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

func (a *apiService) EventsEventsStream(ctx context.Context, params api.EventsEventsStreamParams) (*api.EventsEventsStreamOKHeaders, error) {
	return nil, nil
}

func (e *extendedService) EventsEventsStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	cookie, err := r.Cookie(authCookieName)
	if err != nil {
		http.Error(w, "missing token or authash", http.StatusUnauthorized)
		return
	}
	user, err := auth.VerifyUser(r.Context(), e.api.db, e.api.cache, e.api.cnf.JWT.Secret, cookie.Value)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	userId, _ := strconv.ParseInt(user.Subject, 10, 64)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	eventChan := e.api.events.Subscribe(userId)
	defer e.api.events.Unsubscribe(userId, eventChan)
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-eventChan:
			if !ok {
				return
			}
			src := event.Source.Data()
			if src == nil {
				continue
			}
			eventData := api.Event{
				ID:        event.ID,
				Type:      event.Type,
				CreatedAt: event.CreatedAt,
				Source: api.Source{
					ID:           src.ID,
					Type:         api.SourceType(src.Type),
					Name:         src.Name,
					ParentId:     src.ParentID,
					DestParentId: api.NewOptString(src.DestParentID),
				},
			}

			jsonData, _ := eventData.MarshalJSON()
			fmt.Fprintf(w, "data: %s\n\n", jsonData)
			flusher.Flush()

		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
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

func NewApiService(db *gorm.DB,
	cnf *config.ServerCmdConfig,
	cache cache.Cacher,
	botSelector tgc.BotSelector,
	events events.EventBroadcaster,
	clientPool *tgc.ClientPool) *apiService {

	return &apiService{
		db:             db,
		cnf:            cnf,
		cache:          cache,
		botSelector:    botSelector,
		events:         events,
		channelManager: tgc.NewChannelManager(db, cache, &cnf.TG),
		clientPool:     clientPool,
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

	case api.EventsEventsStreamOperation:
		m.srv.EventsEventsStream(w, r)
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
