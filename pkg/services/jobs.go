package services

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/go-faster/jx"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/queue"
)

type jobClient interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
	JobList(ctx context.Context, params *river.JobListParams) (*river.JobListResult, error)
	JobGet(ctx context.Context, id int64) (*rivertype.JobRow, error)
	JobCancel(ctx context.Context, jobID int64) (*rivertype.JobRow, error)
}

type genericJobArgs struct {
	kind    string
	payload map[string]any
}

func (a genericJobArgs) Kind() string { return a.kind }

func (a genericJobArgs) MarshalJSON() ([]byte, error) { return json.Marshal(a.payload) }

func (a *apiService) JobsList(ctx context.Context, params api.JobsListParams) (*api.JobList, error) {
	userID := auth.User(ctx)
	limit := params.Limit.Or(50)
	listParams := river.NewJobListParams().
		First(limit).
		Where("jsonb_path_query_first(args, @json_path) = @json_val",
			river.NamedArgs{"json_path": "$.userId", "json_val": userID}).
		OrderBy(river.JobListOrderByID, river.SortOrderDesc)

	if params.State.IsSet() {
		listParams = listParams.States(toRiverJobState(params.State.Value))
	}

	if params.Cursor.IsSet() && params.Cursor.Value != "" {
		cursor := &river.JobListCursor{}
		if err := cursor.UnmarshalText([]byte(params.Cursor.Value)); err != nil {
			return nil, &apiError{err: errors.New("invalid cursor"), code: 400}
		}
		listParams = listParams.After(cursor)
	}

	res, err := a.jobs.JobList(ctx, listParams)
	if err != nil {
		return nil, &apiError{err: err}
	}

	items := utils.Map(res.Jobs, func(row *rivertype.JobRow) api.JobStatus { return *toJobStatus(row) })
	meta := api.Meta{}
	if len(items) == limit {
		if cursor, err := river.JobListCursorFromJob(res.Jobs[len(res.Jobs)-1]).MarshalText(); err == nil {
			meta.NextCursor = api.NewOptString(string(cursor))
		}
	}

	return &api.JobList{Items: items, Meta: meta}, nil
}

func (a *apiService) JobsInsert(ctx context.Context, req *api.JobInsertRequest) (*api.JobStatus, error) {
	if a.jobs == nil {
		return nil, &apiError{err: errors.New("jobs service is not configured"), code: 503}
	}

	userID := auth.User(ctx)
	if userID == 0 {
		return nil, &apiError{err: errors.New("unauthorized"), code: 401}
	}

	if !isAllowedInsertKind(req.Kind) {
		return nil, &apiError{err: errors.New("unknown job kind"), code: 400}
	}

	insertOpts := &river.InsertOpts{
		UniqueOpts: river.UniqueOpts{ByArgs: true},
	}
	if req.Queue.IsSet() {
		insertOpts.Queue = req.Queue.Value
	}
	if req.ScheduledAt.IsSet() {
		insertOpts.ScheduledAt = req.ScheduledAt.Value
	}
	if req.MaxAttempts.IsSet() {
		insertOpts.MaxAttempts = req.MaxAttempts.Value
	}

	payload := map[string]any{}
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &payload); err != nil {
			return nil, &apiError{err: errors.New("invalid args"), code: 400}
		}
	}
	payload["userId"] = userID

	insertRes, err := a.jobs.Insert(ctx, genericJobArgs{kind: req.Kind, payload: payload}, insertOpts)
	if err != nil {
		return nil, &apiError{err: err}
	}

	return toJobStatus(insertRes.Job), nil
}

func isAllowedInsertKind(kind string) bool {
	switch kind {
	case queue.JobKindSyncRun:
		return true
	default:
		return false
	}
}

func (a *apiService) JobsGet(ctx context.Context, params api.JobsGetParams) (*api.JobStatus, error) {
	if a.jobs == nil {
		return nil, &apiError{err: errors.New("jobs service is not configured"), code: 503}
	}

	row, err := a.jobs.JobGet(ctx, params.ID)
	if err != nil {
		if errors.Is(err, rivertype.ErrNotFound) {
			return nil, &apiError{err: errors.New("job not found"), code: 404}
		}
		return nil, &apiError{err: err}
	}

	if !jobOwnedByUser(row, auth.User(ctx)) {
		return nil, &apiError{err: errors.New("job not found"), code: 404}
	}

	return toJobStatus(row), nil
}

func (a *apiService) JobsCancel(ctx context.Context, params api.JobsCancelParams) error {
	if a.jobs == nil {
		return &apiError{err: errors.New("jobs service is not configured"), code: 503}
	}

	row, err := a.jobs.JobGet(ctx, params.ID)
	if err != nil {
		if errors.Is(err, rivertype.ErrNotFound) {
			return &apiError{err: errors.New("job not found"), code: 404}
		}
		return &apiError{err: err}
	}

	if !jobOwnedByUser(row, auth.User(ctx)) {
		return &apiError{err: errors.New("job not found"), code: 404}
	}

	if _, err := a.jobs.JobCancel(ctx, params.ID); err != nil {
		if errors.Is(err, rivertype.ErrNotFound) {
			return &apiError{err: errors.New("job not found"), code: 404}
		}
		return &apiError{err: err}
	}

	return nil
}

func jobOwnedByUser(row *rivertype.JobRow, userID int64) bool {
	if userID == 0 {
		return false
	}
	var encoded struct {
		UserID int64 `json:"userId"`
	}
	if err := json.Unmarshal(row.EncodedArgs, &encoded); err != nil {
		return false
	}
	return encoded.UserID == userID
}

func toJobStatus(row *rivertype.JobRow) *api.JobStatus {
	status := &api.JobStatus{
		ID:          row.ID,
		Kind:        row.Kind,
		Queue:       row.Queue,
		State:       toAPIJobState(row.State),
		Attempt:     row.Attempt,
		MaxAttempts: row.MaxAttempts,
		CreatedAt:   row.CreatedAt,
	}

	if !row.ScheduledAt.IsZero() {
		status.ScheduledAt = api.NewOptDateTime(row.ScheduledAt)
	}
	if row.AttemptedAt != nil {
		status.StartedAt = api.NewOptDateTime(*row.AttemptedAt)
	}
	if row.FinalizedAt != nil {
		status.FinalizedAt = api.NewOptDateTime(*row.FinalizedAt)
	}

	if outputRaw := row.Output(); len(outputRaw) > 0 {
		var output api.JobOutput
		if err := output.UnmarshalJSON(outputRaw); err != nil {
			output.Data = jx.Raw(outputRaw)
		}
		status.Output = api.NewOptJobOutput(output)
	}

	if len(row.Errors) > 0 {
		status.Errors = make([]api.JobError, 0, len(row.Errors))
		for _, attemptErr := range row.Errors {
			item := api.JobError{Message: attemptErr.Error}
			if attemptErr.Trace != "" {
				item.Detail = api.NewOptString(attemptErr.Trace)
			}
			status.Errors = append(status.Errors, item)
		}
	}

	return status
}

func toAPIJobState(state rivertype.JobState) api.JobState {
	switch state {
	case rivertype.JobStateAvailable:
		return api.JobStateAvailable
	case rivertype.JobStateRunning:
		return api.JobStateRunning
	case rivertype.JobStateScheduled:
		return api.JobStateScheduled
	case rivertype.JobStateRetryable:
		return api.JobStateRetryable
	case rivertype.JobStateCompleted:
		return api.JobStateCompleted
	case rivertype.JobStateCancelled:
		return api.JobStateCancelled
	case rivertype.JobStateDiscarded:
		return api.JobStateDiscarded
	default:
		return api.JobStateRetryable
	}
}

func toRiverJobState(state api.JobState) rivertype.JobState {
	switch state {
	case api.JobStateAvailable:
		return rivertype.JobStateAvailable
	case api.JobStateRunning:
		return rivertype.JobStateRunning
	case api.JobStateScheduled:
		return rivertype.JobStateScheduled
	case api.JobStateRetryable:
		return rivertype.JobStateRetryable
	case api.JobStateCompleted:
		return rivertype.JobStateCompleted
	case api.JobStateCancelled:
		return rivertype.JobStateCancelled
	case api.JobStateDiscarded:
		return rivertype.JobStateDiscarded
	default:
		return rivertype.JobStateAvailable
	}
}
