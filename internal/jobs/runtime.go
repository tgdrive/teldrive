package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/divyam234/riverpro"
	"github.com/divyam234/riverpro/driver/riverpropgxv5"
	"github.com/divyam234/riverpro/riverencrypt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/tgdrive/teldrive/v2/internal/bots"
	"github.com/tgdrive/teldrive/v2/internal/catalog"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	"github.com/tgdrive/teldrive/v2/internal/transfer"
	"github.com/tgdrive/teldrive/v2/internal/uploads"
)

const (
	uploadCleanupPeriodicID           = "teldrive-upload-cleanup"
	eventCleanupPeriodicID            = "teldrive-user-event-cleanup"
	trashCleanupPeriodicID            = "teldrive-trash-cleanup"
	purgePeriodicID                   = "teldrive-pending-file-purge"
	orphanCleanupPeriodicID           = "teldrive-orphaned-telegram-part-cleanup"
	uploadCleanupDefaultCron          = "@every 12h"
	eventCleanupDefaultCron           = "0 0 * * *"
	eventCleanupDefaultRetention      = "48h"
	trashCleanupDefaultCron           = "@every 12h"
	pendingDeletionCleanupDefaultCron = "@every 12h"
	orphanCleanupDefaultCron          = "@every 336h"
	maintenanceTimezone               = "UTC"
	maintenanceWorkers                = 2
	cleanupPeriodicID                 = uploadCleanupPeriodicID // deprecated compatibility name
)

var ErrRuntimeNotConfigured = errors.New("job runtime is not configured")

type Runtime struct {
	client               *riverpro.Client[pgx.Tx]
	pool                 *pgxpool.Pool
	schema               string
	purgeEnabled         bool
	orphanCleanupEnabled bool
	botProvisionEnabled  bool
	uploadEnabled        bool
	mu                   sync.Mutex
	started              bool
	cancel               context.CancelFunc
}

func NewRuntime(pool *pgxpool.Pool, storage telegramstore.Storage, purgeServices ...PurgeService) (*Runtime, error) {
	return NewRuntimeWithSchema(pool, storage, "teldrive", purgeServices...)
}

func NewRuntimeWithSchema(pool *pgxpool.Pool, storage telegramstore.Storage, schema string, purgeServices ...PurgeService) (*Runtime, error) {
	return newRuntimeWithSchema(pool, storage, schema, nil, nil, 7*24*time.Hour, nil, purgeServices)
}

func NewRuntimeWithSchemaAndBotProvision(pool *pgxpool.Pool, storage telegramstore.Storage, schema string, botService *bots.Service, encryptor riverencrypt.Encryptor, uploadSessionTTL time.Duration, purgeServices ...PurgeService) (*Runtime, error) {
	return newRuntimeWithSchema(pool, storage, schema, botService, encryptor, uploadSessionTTL, nil, purgeServices)
}

type UploaderServices struct {
	Catalog          *catalog.Service
	Uploads          *uploads.Service
	Pipeline         *transfer.Pipeline
	HTTPClient       *http.Client
	ActiveKeyVersion int32
	LocalImportRoots []string
}

func NewRuntimeWithServices(pool *pgxpool.Pool, storage telegramstore.Storage, schema string, botService *bots.Service, encryptor riverencrypt.Encryptor, uploadSessionTTL time.Duration, uploader UploaderServices, purgeServices ...PurgeService) (*Runtime, error) {
	return newRuntimeWithSchema(pool, storage, schema, botService, encryptor, uploadSessionTTL, &uploader, purgeServices)
}

func newRuntimeWithSchema(pool *pgxpool.Pool, storage telegramstore.Storage, schema string, botService *bots.Service, encryptor riverencrypt.Encryptor, uploadSessionTTL time.Duration, uploader *UploaderServices, purgeServices []PurgeService) (*Runtime, error) {
	if pool == nil || storage == nil {
		return nil, ErrRuntimeNotConfigured
	}
	workers := river.NewWorkers()
	if err := river.AddWorkerSafely(workers, NewUploadCleanupWorker(pool, storage)); err != nil {
		return nil, fmt.Errorf("register upload cleanup worker: %w", err)
	}
	if err := river.AddWorkerSafely(workers, NewEventCleanupWorker(pool)); err != nil {
		return nil, fmt.Errorf("register event cleanup worker: %w", err)
	}
	var purgeService PurgeService
	if len(purgeServices) > 0 {
		purgeService = purgeServices[0]
	}
	if purgeService != nil {
		if err := river.AddWorkerSafely(workers, NewPendingFilePurgeWorker(pool, purgeService)); err != nil {
			return nil, fmt.Errorf("register purge worker: %w", err)
		}
		if err := river.AddWorkerSafely(workers, NewTrashCleanupWorker(pool, purgeService)); err != nil {
			return nil, fmt.Errorf("register trash cleanup worker: %w", err)
		}
	}
	lister, orphanCleanupEnabled := storage.(telegramstore.DocumentMessageLister)
	if orphanCleanupEnabled {
		minimumAge := 7 * 24 * time.Hour
		if uploadSessionTTL > 0 {
			minimumAge = uploadSessionTTL
		}
		if err := river.AddWorkerSafely(workers, NewOrphanedTelegramPartsCleanupWorker(pool, storage, lister, minimumAge)); err != nil {
			return nil, fmt.Errorf("register orphan cleanup worker: %w", err)
		}
	}
	inviter, hasInviter := storage.(telegramstore.BotInviter)
	botProvisionEnabled := hasInviter && botService != nil && encryptor != nil
	if botProvisionEnabled {
		if err := river.AddWorkerSafely(workers, NewBotProvisionWorker(pool, botService, inviter)); err != nil {
			return nil, fmt.Errorf("register bot provisioning worker: %w", err)
		}
	}
	uploadEnabled := uploader != nil && uploader.Catalog != nil && uploader.Uploads != nil && uploader.Pipeline != nil
	if uploadEnabled {
		if uploader.HTTPClient == nil {
			uploader.HTTPClient = NewUploadHTTPClient()
		}
		if err := river.AddWorkerSafely(workers, NewUploadBatchWorker(uploader.HTTPClient, uploader.Catalog, uploader.LocalImportRoots)); err != nil {
			return nil, fmt.Errorf("register upload batch worker: %w", err)
		}
		if err := river.AddWorkerSafely(workers, NewUploadSourceWorker(pool, uploader.Catalog, uploader.Uploads, uploader.Pipeline, uploader.HTTPClient, uploader.ActiveKeyVersion)); err != nil {
			return nil, fmt.Errorf("register upload source worker: %w", err)
		}
	}
	riverConfig := river.Config{
		Schema:  schema,
		Workers: workers,
		Queues: map[string]river.QueueConfig{
			CleanupQueue: {MaxWorkers: maintenanceWorkers},
			UploadQueue:  {MaxWorkers: 2},
		},
		SoftStopTimeout: 30 * time.Second,
	}
	if botProvisionEnabled {
		riverConfig.Hooks = append(riverConfig.Hooks, riverencrypt.NewEncryptHookConfig(&riverencrypt.EncryptHookConfig{
			Encryptor:       encryptor,
			JobKindsInclude: []string{BotProvisionKind},
		}))
	}
	client, err := riverpro.NewClient(riverpropgxv5.New(pool), &riverpro.Config{
		Config: riverConfig,
		DurablePeriodicJobs: riverpro.DurablePeriodicJobsConfig{
			Enabled:      true,
			PollInterval: time.Second,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create RiverPro client: %w", err)
	}
	return &Runtime{
		client: client, pool: pool, schema: schema,
		purgeEnabled: purgeService != nil, orphanCleanupEnabled: orphanCleanupEnabled, botProvisionEnabled: botProvisionEnabled, uploadEnabled: uploadEnabled,
	}, nil
}

func (r *Runtime) Start(ctx context.Context) error {
	if r == nil || r.client == nil {
		return ErrRuntimeNotConfigured
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return nil
	}
	uploadCleanupArgs, err := json.Marshal(UploadCleanupSweepArgs{})
	if err != nil {
		return fmt.Errorf("marshal upload cleanup periodic args: %w", err)
	}
	if _, err := r.client.PeriodicJobInsert(ctx, &riverpro.PeriodicJobInsertOpts{
		ID:          uploadCleanupPeriodicID,
		Kind:        UploadCleanupSweepKind,
		Args:        uploadCleanupArgs,
		Queue:       CleanupQueue,
		Priority:    2,
		MaxAttempts: 3,
		Schedule: &riverpro.PeriodicJobSchedule{
			CronExpression: uploadCleanupDefaultCron,
			CronTimezone:   maintenanceTimezone,
		},
	}); err != nil && !errors.Is(err, riverpro.ErrPeriodicJobAlreadyExists) {
		return fmt.Errorf("upsert upload cleanup periodic job: %w", err)
	}
	eventCleanupArgs, err := json.Marshal(EventCleanupArgs{Retention: eventCleanupDefaultRetention})
	if err != nil {
		return fmt.Errorf("marshal event cleanup periodic args: %w", err)
	}
	if _, err := r.client.PeriodicJobInsert(ctx, &riverpro.PeriodicJobInsertOpts{
		ID:          eventCleanupPeriodicID,
		Kind:        EventCleanupKind,
		Args:        eventCleanupArgs,
		Queue:       CleanupQueue,
		Priority:    2,
		MaxAttempts: 3,
		Schedule: &riverpro.PeriodicJobSchedule{
			CronExpression: eventCleanupDefaultCron,
			CronTimezone:   maintenanceTimezone,
		},
	}); err != nil && !errors.Is(err, riverpro.ErrPeriodicJobAlreadyExists) {
		return fmt.Errorf("upsert event cleanup periodic job: %w", err)
	}
	if r.purgeEnabled {
		trashCleanupArgs, err := json.Marshal(TrashCleanupSweepArgs{Retention: "720h"})
		if err != nil {
			return fmt.Errorf("marshal trash cleanup periodic args: %w", err)
		}
		if _, err := r.client.PeriodicJobInsert(ctx, &riverpro.PeriodicJobInsertOpts{
			ID:          trashCleanupPeriodicID,
			Kind:        TrashCleanupSweepKind,
			Args:        trashCleanupArgs,
			Queue:       CleanupQueue,
			Priority:    1,
			MaxAttempts: 3,
			Schedule: &riverpro.PeriodicJobSchedule{
				CronExpression: trashCleanupDefaultCron,
				CronTimezone:   maintenanceTimezone,
			},
		}); err != nil && !errors.Is(err, riverpro.ErrPeriodicJobAlreadyExists) {
			return fmt.Errorf("upsert trash cleanup periodic job: %w", err)
		}

		purgeArgs, err := json.Marshal(PurgeSweepArgs{})
		if err != nil {
			return fmt.Errorf("marshal purge periodic args: %w", err)
		}
		if _, err := r.client.PeriodicJobInsert(ctx, &riverpro.PeriodicJobInsertOpts{
			ID:          purgePeriodicID,
			Kind:        PurgeSweepKind,
			Args:        purgeArgs,
			Queue:       PurgeQueue,
			Priority:    1,
			MaxAttempts: 3,
			Schedule: &riverpro.PeriodicJobSchedule{
				CronExpression: pendingDeletionCleanupDefaultCron,
				CronTimezone:   maintenanceTimezone,
			},
		}); err != nil && !errors.Is(err, riverpro.ErrPeriodicJobAlreadyExists) {
			return fmt.Errorf("upsert purge periodic job: %w", err)
		}
	}
	if r.orphanCleanupEnabled {
		orphanCleanupArgs, err := json.Marshal(OrphanCleanupArgs{})
		if err != nil {
			return fmt.Errorf("marshal orphan cleanup periodic args: %w", err)
		}
		if _, err := r.client.PeriodicJobInsert(ctx, &riverpro.PeriodicJobInsertOpts{
			ID:          orphanCleanupPeriodicID,
			Kind:        OrphanCleanupKind,
			Args:        orphanCleanupArgs,
			Queue:       CleanupQueue,
			Priority:    3,
			MaxAttempts: 3,
			Schedule: &riverpro.PeriodicJobSchedule{
				CronExpression: orphanCleanupDefaultCron,
				CronTimezone:   maintenanceTimezone,
			},
		}); err != nil && !errors.Is(err, riverpro.ErrPeriodicJobAlreadyExists) {
			return fmt.Errorf("upsert orphan cleanup periodic job: %w", err)
		}
	}
	runCtx, cancel := context.WithCancel(ctx)
	if err := r.client.Start(runCtx); err != nil {
		cancel()
		return fmt.Errorf("start RiverPro client: %w", err)
	}
	r.cancel = cancel
	r.started = true
	return nil
}

func (r *Runtime) Stop(ctx context.Context) error {
	if r == nil || r.client == nil {
		return ErrRuntimeNotConfigured
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.started {
		return nil
	}
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	if err := r.client.Stop(ctx); err != nil {
		return fmt.Errorf("stop RiverPro client: %w", err)
	}
	r.started = false
	return nil
}

func (r *Runtime) InsertUploadCleanup(ctx context.Context) error {
	if r == nil || r.client == nil {
		return ErrRuntimeNotConfigured
	}
	if _, err := r.client.Insert(ctx, UploadCleanupSweepArgs{}, nil); err != nil {
		return fmt.Errorf("insert upload cleanup sweep: %w", err)
	}
	return nil
}

// InsertCleanup is kept for callers using the previous generic name.
func (r *Runtime) InsertCleanup(ctx context.Context) error {
	return r.InsertUploadCleanup(ctx)
}

func (r *Runtime) InsertPurge(ctx context.Context) error {
	if r == nil || r.client == nil || !r.purgeEnabled {
		return ErrRuntimeNotConfigured
	}
	if _, err := r.client.Insert(ctx, PurgeSweepArgs{}, nil); err != nil {
		return fmt.Errorf("insert purge sweep: %w", err)
	}
	return nil
}

func (r *Runtime) InsertBotProvision(ctx context.Context, userID int64, botIDs []int64) (string, error) {
	if r == nil || r.client == nil || !r.botProvisionEnabled || userID <= 0 {
		return "", ErrRuntimeNotConfigured
	}
	botIDs = normalizedBotIDs(botIDs)
	if len(botIDs) == 0 {
		return "", nil
	}
	result, err := r.client.Insert(ctx, BotProvisionArgs{UserID: userID, BotIDs: botIDs}, nil)
	if err != nil {
		return "", fmt.Errorf("insert bot provisioning job: %w", err)
	}
	return fmt.Sprintf("%d", result.Job.ID), nil
}

func (r *Runtime) InsertUploadBatch(ctx context.Context, args UploadBatchArgs) (Job, error) {
	if r == nil || r.client == nil || !r.uploadEnabled || args.UserID <= 0 || len(args.Sources) == 0 {
		return Job{}, ErrRuntimeNotConfigured
	}
	if strings.TrimSpace(args.BatchID) == "" {
		args.BatchID = uuid.NewString()
	}
	result, err := r.client.Insert(ctx, args, nil)
	if err != nil {
		return Job{}, fmt.Errorf("insert upload batch: %w", err)
	}
	return jobFromRiver(result.Job), nil
}
