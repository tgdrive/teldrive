package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-faster/jx"
	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/robfig/cron/v3"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/auth"
	internalduration "github.com/tgdrive/teldrive/internal/duration"
	"github.com/tgdrive/teldrive/pkg/queue"
	"github.com/tgdrive/teldrive/pkg/repositories"
)

const (
	periodicJobKindSyncRun          = "sync.run"
	periodicJobKindCleanOldEvents   = "clean.old_events"
	periodicJobKindCleanStaleUpload = "clean.stale_uploads"
	periodicJobKindCleanPendingFile = "clean.pending_files"
	defaultOldEventsRetention       = "5d"
	defaultStaleUploadRetention     = "1d"
)

type periodicJobRow struct {
	ID             string
	UserID         int64
	Name           string
	Kind           string
	Args           repositories.PeriodicJobArgs
	CronExpression string
	Enabled        bool
	System         bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type periodicJobPreset struct {
	Name           string
	Kind           string
	CronExpression string
	Args           repositories.PeriodicJobArgs
	System         bool
}

func (a *apiService) PeriodicJobsList(ctx context.Context) ([]api.PeriodicJobSummary, error) {
	userID := auth.User(ctx)
	if err := a.ensureDefaultPeriodicJobs(ctx, userID); err != nil {
		return nil, err
	}

	rows, err := a.repo.PeriodicJobs.ListByUserID(ctx, userID)
	if err != nil {
		return nil, &apiError{err: err}
	}

	out := make([]api.PeriodicJobSummary, 0, len(rows))
	for _, itemRow := range rows {
		item, err := toAPIPeriodicJobSummary(fromPeriodicJobModel(itemRow))
		if err != nil {
			return nil, &apiError{err: err}
		}
		out = append(out, *item)
	}
	return out, nil
}

func (a *apiService) PeriodicJobsCreate(ctx context.Context, req *api.PeriodicJobCreate) (*api.PeriodicJobDetail, error) {
	userID := auth.User(ctx)
	if err := validatePeriodicSyncArgs(req.Args); err != nil {
		return nil, &apiError{err: err, code: 400}
	}
	cronExpr := strings.TrimSpace(req.CronExpression)
	if err := validatePeriodicCron(cronExpr); err != nil {
		return nil, &apiError{err: err, code: 400}
	}
	enabled := true
	if req.Enabled.IsSet() {
		enabled = req.Enabled.Value
	}
	id := uuid.NewString()
	jobModel := &repositories.PeriodicJob{
		ID:             uuid.MustParse(id),
		UserID:         userID,
		Name:           req.Name,
		Kind:           periodicJobKindSyncRun,
		Args:           syncRunArgsFromAPI(req.Args),
		CronExpression: cronExpr,
		Enabled:        enabled,
		System:         false,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	err := a.repo.PeriodicJobs.Create(ctx, jobModel)
	if err != nil {
		return nil, &apiError{err: err}
	}

	row, err := a.getPeriodicJobRow(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if row.Enabled {
		if err := a.syncPeriodicJobRegistry(row, periodicRegistryActionAdd); err != nil {
			return nil, err
		}
	}
	return a.PeriodicJobsGet(ctx, api.PeriodicJobsGetParams{ID: api.UUID(uuid.MustParse(id))})
}

func (a *apiService) PeriodicJobsGet(ctx context.Context, params api.PeriodicJobsGetParams) (*api.PeriodicJobDetail, error) {
	row, err := a.getPeriodicJobRow(ctx, uuid.UUID(params.ID).String(), auth.User(ctx))
	if err != nil {
		return nil, err
	}
	item, convErr := toAPIPeriodicJobDetail(row)
	if convErr != nil {
		return nil, &apiError{err: convErr}
	}
	return item, nil
}

func (a *apiService) PeriodicJobsUpdate(ctx context.Context, req *api.PeriodicJobUpdate, params api.PeriodicJobsUpdateParams) (*api.PeriodicJobDetail, error) {
	row, err := a.getPeriodicJobRow(ctx, uuid.UUID(params.ID).String(), auth.User(ctx))
	if err != nil {
		return nil, err
	}
	isSyncJob := row.Kind == periodicJobKindSyncRun

	name := row.Name
	if req.Name.IsSet() {
		if !isSyncJob {
			return nil, &apiError{err: errors.New("maintenance job name cannot be changed"), code: 400}
		}
		name = req.Name.Value
	}

	cronExpr := row.CronExpression
	if req.CronExpression.IsSet() {
		cronExpr = strings.TrimSpace(req.CronExpression.Value)
	}
	if err := validatePeriodicCron(cronExpr); err != nil {
		return nil, &apiError{err: err, code: 400}
	}

	enabled := row.Enabled
	if req.Enabled.IsSet() {
		enabled = req.Enabled.Value
	}

	currentSyncArgs, ok := row.Args.(repositories.SyncRunPeriodicArgs)
	if req.Args.IsSet() {
		updatedArgs, err := updatePeriodicJobArgs(row, req.Args.Value, isSyncJob, ok, currentSyncArgs)
		if err != nil {
			return nil, err
		}
		row.Args = updatedArgs
	}

	err = a.repo.PeriodicJobs.Update(ctx, uuid.MustParse(row.ID), row.UserID, repositories.PeriodicJob{
		Kind:           row.Kind,
		Name:           name,
		CronExpression: cronExpr,
		Enabled:        enabled,
		Args:           row.Args,
		UpdatedAt:      time.Now().UTC(),
	})
	if err != nil {
		return nil, &apiError{err: err}
	}

	row.Name = name
	row.CronExpression = cronExpr
	row.Enabled = enabled

	if err := a.syncPeriodicJobRegistry(row, periodicRegistryActionReplace); err != nil {
		return nil, err
	}
	return a.PeriodicJobsGet(ctx, api.PeriodicJobsGetParams(params))
}

func (a *apiService) PeriodicJobsDelete(ctx context.Context, params api.PeriodicJobsDeleteParams) error {
	row, err := a.getPeriodicJobRow(ctx, uuid.UUID(params.ID).String(), auth.User(ctx))
	if err != nil {
		return err
	}
	if row.Kind != periodicJobKindSyncRun {
		return &apiError{err: errors.New("maintenance jobs cannot be deleted"), code: 400}
	}
	if a.periodicJobs != nil {
		a.periodicJobs.RemoveByID(periodicRegistryID(row.ID))
	}
	err = a.repo.PeriodicJobs.Delete(ctx, uuid.MustParse(row.ID), row.UserID)
	if err != nil {
		return &apiError{err: err}
	}
	return nil
}

func (a *apiService) PeriodicJobsEnable(ctx context.Context, params api.PeriodicJobsEnableParams) error {
	return a.setPeriodicJobEnabled(ctx, uuid.UUID(params.ID).String(), true)
}

func (a *apiService) PeriodicJobsDisable(ctx context.Context, params api.PeriodicJobsDisableParams) error {
	return a.setPeriodicJobEnabled(ctx, uuid.UUID(params.ID).String(), false)
}

func (a *apiService) PeriodicJobsRun(ctx context.Context, params api.PeriodicJobsRunParams) (*api.JobStatus, error) {
	row, err := a.getPeriodicJobRow(ctx, uuid.UUID(params.ID).String(), auth.User(ctx))
	if err != nil {
		return nil, err
	}
	job, err := a.insertPeriodicRuntimeJob(ctx, row)
	if err != nil {
		return nil, err
	}
	return toJobStatus(job), nil
}

func (a *apiService) PeriodicJobsTestConnection(ctx context.Context, req *api.SyncArgs) error {
	if err := validatePeriodicSyncArgs(*req); err != nil {
		return &apiError{err: err, code: 400}
	}
	headers := map[string]string{}
	if req.Headers.IsSet() {
		headers = req.Headers.Value
	}
	proxy := ""
	if req.Proxy.IsSet() {
		proxy = strings.TrimSpace(req.Proxy.Value)
	}
	fs, err := remoteFSForSource(req.Source)
	if err != nil {
		return &apiError{err: err, code: 400}
	}
	_, err = fs.List(ctx, "", headers, proxy)
	if err != nil {
		return &apiError{err: err, code: 400}
	}
	return nil
}

func (a *apiService) setPeriodicJobEnabled(ctx context.Context, id string, enabled bool) error {
	row, err := a.getPeriodicJobRow(ctx, id, auth.User(ctx))
	if err != nil {
		return err
	}
	err = a.repo.PeriodicJobs.SetEnabled(ctx, uuid.MustParse(row.ID), row.UserID, enabled, time.Now().UTC())
	if err != nil {
		return &apiError{err: err}
	}
	row.Enabled = enabled
	action := periodicRegistryActionAdd
	if !enabled {
		action = periodicRegistryActionRemove
	}
	return a.syncPeriodicJobRegistry(row, action)
}

func (a *apiService) ensureDefaultPeriodicJobs(ctx context.Context, userID int64) error {
	if userID == 0 {
		return nil
	}
	for _, preset := range defaultPeriodicJobPresets() {
		row, err := a.getPeriodicJobByName(ctx, userID, preset.Name)
		if err != nil {
			var apiErr *apiError
			if errors.As(err, &apiErr) && apiErr.code == 404 {
				created, createErr := a.insertPeriodicPreset(ctx, userID, preset)
				if createErr != nil {
					return createErr
				}
				if created.Enabled {
					if scheduleErr := a.syncPeriodicJobRegistry(created, periodicRegistryActionAdd); scheduleErr != nil {
						return scheduleErr
					}
				}
				continue
			}
			return err
		}
		if updated, updateErr := a.ensurePeriodicPresetArgs(ctx, row); updateErr != nil {
			return updateErr
		} else if updated != nil {
			row = updated
		}
		if row.Enabled {
			if err := a.syncPeriodicJobRegistry(row, periodicRegistryActionAdd); err != nil {
				return err
			}
		}
	}
	return nil
}

func defaultPeriodicJobPresets() []periodicJobPreset {
	return []periodicJobPreset{
		{Name: "Clean Old Events", Kind: periodicJobKindCleanOldEvents, CronExpression: "0 */12 * * *", Args: defaultCleanOldEventsPeriodicArgs(), System: true},
		{Name: "Clean Stale Uploads", Kind: periodicJobKindCleanStaleUpload, CronExpression: "0 */12 * * *", Args: defaultCleanStaleUploadsPeriodicArgs(), System: true},
		{Name: "Clean Pending Files", Kind: periodicJobKindCleanPendingFile, CronExpression: "0 * * * *", Args: repositories.CleanPendingFilesPeriodicArgs{}, System: true},
	}
}

func defaultCleanOldEventsPeriodicArgs() repositories.CleanOldEventsPeriodicArgs {
	return repositories.CleanOldEventsPeriodicArgs{Retention: defaultOldEventsRetention}
}

func defaultCleanStaleUploadsPeriodicArgs() repositories.CleanStaleUploadsPeriodicArgs {
	return repositories.CleanStaleUploadsPeriodicArgs{Retention: defaultStaleUploadRetention}
}

func normalizePeriodicJobArgs(kind string, args repositories.PeriodicJobArgs) repositories.PeriodicJobArgs {
	switch kind {
	case periodicJobKindCleanOldEvents:
		return normalizeCleanOldEventsPeriodicArgs(args)
	case periodicJobKindCleanStaleUpload:
		return normalizeCleanStaleUploadsPeriodicArgs(args)
	default:
		return args
	}
}

func normalizeCleanOldEventsPeriodicArgs(args repositories.PeriodicJobArgs) repositories.CleanOldEventsPeriodicArgs {
	defaultArgs := defaultCleanOldEventsPeriodicArgs()
	switch v := args.(type) {
	case repositories.CleanOldEventsPeriodicArgs:
		if normalized, ok := normalizeRetentionString(v.Retention); ok {
			return repositories.CleanOldEventsPeriodicArgs{Retention: normalized}
		}
	case *repositories.CleanOldEventsPeriodicArgs:
		if v != nil {
			if normalized, ok := normalizeRetentionString(v.Retention); ok {
				return repositories.CleanOldEventsPeriodicArgs{Retention: normalized}
			}
		}
	}
	return defaultArgs
}

func normalizeCleanStaleUploadsPeriodicArgs(args repositories.PeriodicJobArgs) repositories.CleanStaleUploadsPeriodicArgs {
	defaultArgs := defaultCleanStaleUploadsPeriodicArgs()
	switch v := args.(type) {
	case repositories.CleanStaleUploadsPeriodicArgs:
		if normalized, ok := normalizeRetentionString(v.Retention); ok {
			return repositories.CleanStaleUploadsPeriodicArgs{Retention: normalized}
		}
	case *repositories.CleanStaleUploadsPeriodicArgs:
		if v != nil {
			if normalized, ok := normalizeRetentionString(v.Retention); ok {
				return repositories.CleanStaleUploadsPeriodicArgs{Retention: normalized}
			}
		}
	}
	return defaultArgs
}

func normalizeRetentionString(raw string) (string, bool) {
	d, err := internalduration.ParseDuration(strings.TrimSpace(raw))
	if err != nil || d <= 0 {
		return "", false
	}
	formatted := internalduration.Duration(d)
	return formatted.String(), true
}

func updatePeriodicJobArgs(
	row *periodicJobRow,
	rawArgs api.PeriodicJobUpdateArgs,
	isSyncJob bool,
	hasSyncArgs bool,
	currentSyncArgs repositories.SyncRunPeriodicArgs,
) (repositories.PeriodicJobArgs, error) {
	if isSyncJob {
		if !hasSyncArgs {
			return nil, &apiError{err: errors.New("invalid sync args payload"), code: 400}
		}
		currentArgs := apiSyncArgsFromDomain(currentSyncArgs)
		updatedArgs, err := syncArgsUpdateToArgs(rawArgs, currentArgs)
		if err != nil {
			return nil, &apiError{err: errors.New("invalid sync args payload"), code: 400}
		}
		if err := validatePeriodicSyncArgs(updatedArgs); err != nil {
			return nil, &apiError{err: err, code: 400}
		}
		return syncRunArgsFromAPI(updatedArgs), nil
	}

	maintenanceArgs, err := maintenanceArgsUpdateFromAPI(row.Kind, rawArgs)
	if err != nil {
		return nil, err
	}
	return maintenanceArgs, nil
}

func maintenanceArgsUpdateFromAPI(kind string, rawArgs api.PeriodicJobUpdateArgs) (repositories.PeriodicJobArgs, error) {
	b, err := json.Marshal(rawArgs)
	if err != nil {
		return nil, &apiError{err: err, code: 400}
	}

	switch kind {
	case periodicJobKindCleanOldEvents:
		var args repositories.CleanOldEventsPeriodicArgs
		if err := json.Unmarshal(b, &args); err != nil {
			return nil, &apiError{err: errors.New("invalid maintenance args payload"), code: 400}
		}
		normalized, ok := normalizeRetentionString(args.Retention)
		if !ok {
			return nil, &apiError{err: errors.New("retention must be a valid duration like 1h, 1d, or 5d"), code: 400}
		}
		return repositories.CleanOldEventsPeriodicArgs{Retention: normalized}, nil
	case periodicJobKindCleanStaleUpload:
		var args repositories.CleanStaleUploadsPeriodicArgs
		if err := json.Unmarshal(b, &args); err != nil {
			return nil, &apiError{err: errors.New("invalid maintenance args payload"), code: 400}
		}
		normalized, ok := normalizeRetentionString(args.Retention)
		if !ok {
			return nil, &apiError{err: errors.New("retention must be a valid duration like 1h, 1d, or 5d"), code: 400}
		}
		return repositories.CleanStaleUploadsPeriodicArgs{Retention: normalized}, nil
	case periodicJobKindCleanPendingFile:
		return nil, &apiError{err: errors.New("args cannot be updated for clean.pending_files jobs"), code: 400}
	default:
		return nil, &apiError{err: errors.New("args can only be updated for supported periodic jobs"), code: 400}
	}
}

func (a *apiService) ensurePeriodicPresetArgs(ctx context.Context, row *periodicJobRow) (*periodicJobRow, error) {
	normalizedArgs := normalizePeriodicJobArgs(row.Kind, row.Args)
	if !periodicJobArgsNeedUpdate(row.Kind, row.Args, normalizedArgs) {
		return nil, nil
	}
	updatedAt := time.Now().UTC()
	if err := a.repo.PeriodicJobs.Update(ctx, uuid.MustParse(row.ID), row.UserID, repositories.PeriodicJob{
		Kind:           row.Kind,
		Name:           row.Name,
		CronExpression: row.CronExpression,
		Enabled:        row.Enabled,
		Args:           normalizedArgs,
		UpdatedAt:      updatedAt,
	}); err != nil {
		return nil, &apiError{err: err}
	}
	row.Args = normalizedArgs
	row.UpdatedAt = updatedAt
	return row, nil
}

func periodicJobArgsNeedUpdate(kind string, current, normalized repositories.PeriodicJobArgs) bool {
	switch kind {
	case periodicJobKindCleanOldEvents:
		normalizedArgs, ok := normalized.(repositories.CleanOldEventsPeriodicArgs)
		if !ok {
			return false
		}
		switch v := current.(type) {
		case repositories.CleanOldEventsPeriodicArgs:
			return v.Retention != normalizedArgs.Retention
		case *repositories.CleanOldEventsPeriodicArgs:
			if v == nil {
				return true
			}
			return v.Retention != normalizedArgs.Retention
		default:
			return true
		}
	case periodicJobKindCleanStaleUpload:
		normalizedArgs, ok := normalized.(repositories.CleanStaleUploadsPeriodicArgs)
		if !ok {
			return false
		}
		switch v := current.(type) {
		case repositories.CleanStaleUploadsPeriodicArgs:
			return v.Retention != normalizedArgs.Retention
		case *repositories.CleanStaleUploadsPeriodicArgs:
			if v == nil {
				return true
			}
			return v.Retention != normalizedArgs.Retention
		default:
			return true
		}
	default:
		return false
	}
}

func (a *apiService) insertPeriodicPreset(ctx context.Context, userID int64, preset periodicJobPreset) (*periodicJobRow, error) {
	id := uuid.NewString()
	jobModel := &repositories.PeriodicJob{
		ID:             uuid.MustParse(id),
		UserID:         userID,
		Name:           preset.Name,
		Kind:           preset.Kind,
		Args:           preset.Args,
		CronExpression: preset.CronExpression,
		Enabled:        true,
		System:         preset.System,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	err := a.repo.PeriodicJobs.Create(ctx, jobModel)
	if err != nil {
		return nil, &apiError{err: err}
	}
	return a.getPeriodicJobRow(ctx, id, userID)
}

func (a *apiService) getPeriodicJobByName(ctx context.Context, userID int64, name string) (*periodicJobRow, error) {
	item, err := a.repo.PeriodicJobs.GetByNameAndUserID(ctx, userID, name)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, &apiError{err: errors.New("periodic job not found"), code: 404}
		}
		return nil, &apiError{err: err}
	}
	return fromPeriodicJobModel(*item), nil
}

func (a *apiService) getPeriodicJobRow(ctx context.Context, id string, userID int64) (*periodicJobRow, error) {
	item, err := a.repo.PeriodicJobs.GetByIDAndUserID(ctx, uuid.MustParse(id), userID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, &apiError{err: errors.New("periodic job not found"), code: 404}
		}
		return nil, &apiError{err: err}
	}
	return fromPeriodicJobModel(*item), nil
}

func fromPeriodicJobModel(item repositories.PeriodicJob) *periodicJobRow {
	return &periodicJobRow{
		ID:             item.ID.String(),
		UserID:         item.UserID,
		Name:           item.Name,
		Kind:           item.Kind,
		Args:           normalizePeriodicJobArgs(item.Kind, item.Args),
		CronExpression: item.CronExpression,
		Enabled:        item.Enabled,
		System:         item.System,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

func toAPIPeriodicJobSummary(row *periodicJobRow) (*api.PeriodicJobSummary, error) {
	out := &api.PeriodicJobSummary{
		ID:             api.UUID(uuid.MustParse(row.ID)),
		Name:           row.Name,
		Kind:           api.PeriodicJobKind(row.Kind),
		Enabled:        row.Enabled,
		CronExpression: row.CronExpression,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
	return out, nil
}

func toAPIPeriodicJobDetail(row *periodicJobRow) (*api.PeriodicJobDetail, error) {
	out := &api.PeriodicJobDetail{
		ID:             api.UUID(uuid.MustParse(row.ID)),
		Name:           row.Name,
		Kind:           api.PeriodicJobKind(row.Kind),
		Enabled:        row.Enabled,
		CronExpression: row.CronExpression,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
	if args := toAPIPeriodicJobDetailArgs(row.Args); len(args) > 0 {
		out.Args = api.NewOptPeriodicJobDetailArgs(args)
	}
	return out, nil
}

func validatePeriodicCron(expr string) error {
	if expr == "" {
		return fmt.Errorf("cronExpression is required")
	}
	if _, err := cron.ParseStandard(expr); err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}
	return nil
}

func validatePeriodicSyncArgs(args api.SyncArgs) error {
	if strings.TrimSpace(args.Source) == "" {
		return fmt.Errorf("source is required")
	}
	if strings.TrimSpace(args.DestinationDir) == "" {
		return fmt.Errorf("destinationDir is required")
	}
	if !strings.HasPrefix(args.DestinationDir, "/") {
		return fmt.Errorf("destinationDir must start with '/'")
	}
	return nil
}

type periodicRegistryAction int

const (
	periodicRegistryActionAdd periodicRegistryAction = iota
	periodicRegistryActionReplace
	periodicRegistryActionRemove
)

func periodicRegistryID(id string) string {
	return "periodic-job:" + id
}

func (a *apiService) RegisterPeriodicJobs(ctx context.Context) error {
	if a.periodicJobs == nil {
		return nil
	}
	rows, err := a.repo.PeriodicJobs.ListEnabled(ctx)
	if err != nil {
		return err
	}
	jobs := make([]*river.PeriodicJob, 0, len(rows))
	for _, row := range rows {
		periodicJob, buildErr := a.buildRiverPeriodicJob(fromPeriodicJobModel(row))
		if buildErr != nil {
			return buildErr
		}
		jobs = append(jobs, periodicJob)
	}
	a.periodicJobs.AddMany(jobs)
	return nil
}

func (a *apiService) syncPeriodicJobRegistry(row *periodicJobRow, action periodicRegistryAction) error {
	if a.periodicJobs == nil {
		return nil
	}
	periodicID := periodicRegistryID(row.ID)
	if action == periodicRegistryActionRemove || !row.Enabled {
		a.periodicJobs.RemoveByID(periodicID)
		return nil
	}

	periodicJob, err := a.buildRiverPeriodicJob(row)
	if err != nil {
		return err
	}

	switch action {
	case periodicRegistryActionAdd:
		if _, err := a.periodicJobs.AddSafely(periodicJob); err != nil {
			a.periodicJobs.RemoveByID(periodicID)
			if _, addErr := a.periodicJobs.AddSafely(periodicJob); addErr != nil {
				return &apiError{err: addErr}
			}
		}
	case periodicRegistryActionReplace:
		a.periodicJobs.RemoveByID(periodicID)
		if _, err := a.periodicJobs.AddSafely(periodicJob); err != nil {
			return &apiError{err: err}
		}
	}
	return nil
}

func (a *apiService) buildRiverPeriodicJob(row *periodicJobRow) (*river.PeriodicJob, error) {
	schedule, err := cron.ParseStandard(row.CronExpression)
	if err != nil {
		return nil, &apiError{err: err, code: 400}
	}
	return river.NewPeriodicJob(schedule, func() (river.JobArgs, *river.InsertOpts) {
		args, opts, err := a.runtimePeriodicInsert(row)
		if err != nil {
			return nil, nil
		}
		return args, opts
	}, &river.PeriodicJobOpts{ID: periodicRegistryID(row.ID)}), nil
}

func (a *apiService) insertPeriodicRuntimeJob(ctx context.Context, row *periodicJobRow) (*rivertype.JobRow, error) {
	if a.jobs == nil {
		return nil, &apiError{err: errors.New("jobs service is not configured"), code: 503}
	}
	jobArgs, insertOpts, err := a.runtimePeriodicInsert(row)
	if err != nil {
		return nil, err
	}
	inserted, err := a.jobs.Insert(ctx, jobArgs, insertOpts)
	if err != nil {
		return nil, &apiError{err: err}
	}
	return inserted.Job, nil
}

func (a *apiService) runtimePeriodicInsert(row *periodicJobRow) (river.JobArgs, *river.InsertOpts, error) {
	switch row.Kind {
	case periodicJobKindSyncRun:
		syncArgs, ok := row.Args.(repositories.SyncRunPeriodicArgs)
		if !ok {
			return nil, nil, &apiError{err: errors.New("invalid sync args payload"), code: 400}
		}
		args := apiSyncArgsFromDomain(syncArgs)
		return queue.SyncRunJobArgs{
			UserID:         row.UserID,
			Source:         args.Source,
			DestinationDir: args.DestinationDir,
			Headers:        map[string]string(args.Headers.Or(map[string]string{})),
			Proxy:          args.Proxy.Or(""),
			Filters:        toQueueFilters(args.Filters),
			Options:        toQueueOptions(args.Options),
		}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}, nil
	case periodicJobKindCleanOldEvents:
		oldEventsArgs := normalizeCleanOldEventsPeriodicArgs(row.Args)
		return queue.CleanOldEventsArgs{UserID: row.UserID, Retention: oldEventsArgs.Retention}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}, nil
	case periodicJobKindCleanStaleUpload:
		staleArgs := normalizeCleanStaleUploadsPeriodicArgs(row.Args)
		return queue.CleanStaleUploadsArgs{UserID: row.UserID, Retention: staleArgs.Retention}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}, nil
	case periodicJobKindCleanPendingFile:
		return queue.CleanPendingFilesArgs{UserID: row.UserID}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}}, nil
	default:
		return nil, nil, &apiError{err: fmt.Errorf("unsupported periodic job kind: %s", row.Kind), code: 400}
	}
}

func syncRunArgsFromAPI(v api.SyncArgs) repositories.SyncRunPeriodicArgs {
	out := repositories.SyncRunPeriodicArgs{
		Source:         v.Source,
		DestinationDir: v.DestinationDir,
	}
	if v.Proxy.IsSet() {
		p := v.Proxy.Value
		out.Proxy = &p
	}
	if v.Headers.IsSet() {
		out.Headers = map[string]string(v.Headers.Value)
	}
	if v.Filters.IsSet() {
		f := repositories.SyncFiltersArgs{
			Include:          v.Filters.Value.Include,
			Exclude:          v.Filters.Value.Exclude,
			ExcludeIfPresent: v.Filters.Value.ExcludeIfPresent,
		}
		if v.Filters.Value.MinSize.IsSet() {
			min := v.Filters.Value.MinSize.Value
			f.MinSize = &min
		}
		if v.Filters.Value.MaxSize.IsSet() {
			max := v.Filters.Value.MaxSize.Value
			f.MaxSize = &max
		}
		out.Filters = &f
	}
	if v.Options.IsSet() {
		o := repositories.SyncOptionsArgs{}
		if v.Options.Value.PartSize.IsSet() {
			partSize := v.Options.Value.PartSize.Value
			o.PartSize = &partSize
		}
		if v.Options.Value.Sync.IsSet() {
			sync := v.Options.Value.Sync.Value
			o.Sync = &sync
		}
		out.Options = &o
	}
	return out
}

func apiSyncArgsFromDomain(v repositories.SyncRunPeriodicArgs) api.SyncArgs {
	out := api.SyncArgs{Source: v.Source, DestinationDir: v.DestinationDir}
	if v.Proxy != nil {
		out.Proxy = api.NewOptString(*v.Proxy)
	}
	if len(v.Headers) > 0 {
		out.Headers = api.NewOptSyncArgsHeaders(api.SyncArgsHeaders(v.Headers))
	}
	if v.Filters != nil {
		f := api.SyncFilters{Include: v.Filters.Include, Exclude: v.Filters.Exclude, ExcludeIfPresent: v.Filters.ExcludeIfPresent}
		if v.Filters.MinSize != nil {
			f.MinSize = api.NewOptInt64(*v.Filters.MinSize)
		}
		if v.Filters.MaxSize != nil {
			f.MaxSize = api.NewOptInt64(*v.Filters.MaxSize)
		}
		out.Filters = api.NewOptSyncFilters(f)
	}
	if v.Options != nil {
		o := api.SyncOptions{}
		if v.Options.PartSize != nil {
			o.PartSize = api.NewOptInt64(*v.Options.PartSize)
		}
		if v.Options.Sync != nil {
			o.Sync = api.NewOptBool(*v.Options.Sync)
		}
		out.Options = api.NewOptSyncOptions(o)
	}
	return out
}

func toAPIPeriodicJobDetailArgs(v repositories.PeriodicJobArgs) api.PeriodicJobDetailArgs {
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 || string(b) == "{}" {
		return nil
	}
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(b, &raw); err != nil || len(raw) == 0 {
		return nil
	}
	out := make(api.PeriodicJobDetailArgs, len(raw))
	for k, rv := range raw {
		out[k] = jx.Raw(rv)
	}
	return out
}
