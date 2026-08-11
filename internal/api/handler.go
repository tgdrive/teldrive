package api

import (
	"errors"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/authn"
	"github.com/tgdrive/teldrive/v2/internal/bots"
	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/channels"
	"github.com/tgdrive/teldrive/v2/internal/events"
	"github.com/tgdrive/teldrive/v2/internal/fileops"
	"github.com/tgdrive/teldrive/v2/internal/health"
	"github.com/tgdrive/teldrive/v2/internal/jobs"
	"github.com/tgdrive/teldrive/v2/internal/shares"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	"github.com/tgdrive/teldrive/v2/internal/transfer"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

var ErrOperationUnavailable = errors.New("operation is not implemented")

type Handler struct {
	Catalog                     *catalog.Service
	Uploads                     *uploads.Service
	UploadPipeline              *transfer.Pipeline
	Downloader                  *transfer.Downloader
	Events                      *events.Service
	Health                      *health.Service
	Jobs                        *jobs.Runtime
	Auth                        *authn.Service
	Bots                        *bots.Service
	Channels                    *channels.Service
	FileOps                     *fileops.Service
	Shares                      *shares.Service
	TelegramAccount             telegramstore.Account
	DefaultEncryptionKeyVersion int32
}

func NewHandler(catalogService *catalog.Service, uploadService *uploads.Service, uploadPipeline *transfer.Pipeline, downloader *transfer.Downloader, healthService *health.Service, defaultEncryptionKeyVersion int32, eventService *events.Service) *Handler {
	return &Handler{
		Catalog: catalogService, Uploads: uploadService, UploadPipeline: uploadPipeline,
		Downloader: downloader, Events: eventService, Health: healthService,
		DefaultEncryptionKeyVersion: defaultEncryptionKeyVersion,
	}
}

func (h *Handler) ConfigureDomains(authService *authn.Service, botService *bots.Service, channelService *channels.Service, fileService *fileops.Service, shareService *shares.Service, account telegramstore.Account) *Handler {
	if h != nil {
		h.Auth, h.Bots, h.Channels, h.FileOps, h.Shares = authService, botService, channelService, fileService, shareService
		h.TelegramAccount = account
	}
	return h
}

func (h *Handler) ConfigureJobs(runtime *jobs.Runtime) *Handler {
	if h != nil {
		h.Jobs = runtime
	}
	return h
}

var _ gen.Handler = (*Handler)(nil)
