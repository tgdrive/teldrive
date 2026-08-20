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
	cleanupPeriodicID   = "teldrive-upload-cleanup"
	purgePeriodicID     = "teldrive-pending-file-purge"
	defaultCron         = "*/5 * * * *"
	maintenanceTimezone = "UTC"
	maintenanceWorkers  = 2
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
		return nil, fmt.Errorf("register cleanup worker: %w", err)
	}
	var purgeService PurgeService
	if len(purgeServices) > 0 {
		purgeService = purgeServices[0]
	}
	if purgeService != nil {
		if err := river.AddWorkerSafely(workers, NewPendingFilePurgeWorker(pool, purgeService)); err != nil {
			return nil, fmt.Errorf("register purge worker: %w", err)
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
		if err := river.AddWorkerSafely(workers, NewUploadBatchWorker(uploader.HTTPClient, uploader.Catalog)); err != nil {
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
	cleanupArgs, err := json.Marshal(CleanupSweepArgs{BatchSize: defaultBatchSize})
	if err != nil {
		return fmt.Errorf("marshal cleanup periodic args: %w", err)
	}
	if _, err := r.client.PeriodicJobInsert(ctx, &riverpro.PeriodicJobInsertOpts{
		ID:          cleanupPeriodicID,
		Kind:        CleanupSweepKind,
		Args:        cleanupArgs,
		Queue:       CleanupQueue,
		Priority:    2,
		MaxAttempts: 3,
		Schedule: &riverpro.PeriodicJobSchedule{
			CronExpression: defaultCron,
			CronTimezone:   maintenanceTimezone,
		},
	}); err != nil && !errors.Is(err, riverpro.ErrPeriodicJobAlreadyExists) {
		return fmt.Errorf("upsert cleanup periodic job: %w", err)
	}
	if r.purgeEnabled {
		purgeArgs, err := json.Marshal(PurgeSweepArgs{BatchSize: defaultBatchSize})
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
				CronExpression: defaultCron,
				CronTimezone:   maintenanceTimezone,
			},
		}); err != nil && !errors.Is(err, riverpro.ErrPeriodicJobAlreadyExists) {
			return fmt.Errorf("upsert purge periodic job: %w", err)
		}
	}
	if err := r.client.Start(ctx); err != nil {
		return fmt.Errorf("start RiverPro client: %w", err)
	}
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
	if err := r.client.Stop(ctx); err != nil {
		return fmt.Errorf("stop RiverPro client: %w", err)
	}
	r.started = false
	return nil
}

func (r *Runtime) InsertCleanup(ctx context.Context, batchSize int32) error {
	if r == nil || r.client == nil {
		return ErrRuntimeNotConfigured
	}
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	if _, err := r.client.Insert(ctx, CleanupSweepArgs{BatchSize: batchSize}, nil); err != nil {
		return fmt.Errorf("insert cleanup sweep: %w", err)
	}
	return nil
}

func (r *Runtime) InsertPurge(ctx context.Context, batchSize int32) error {
	if r == nil || r.client == nil || !r.purgeEnabled {
		return ErrRuntimeNotConfigured
	}
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	if _, err := r.client.Insert(ctx, PurgeSweepArgs{BatchSize: batchSize}, nil); err != nil {
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
