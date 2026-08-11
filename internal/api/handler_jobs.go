package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-faster/jx"
	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/jobs"
)

func (h *Handler) ListJobs(ctx context.Context, params gen.ListJobsParams) (gen.ListJobsRes, error) {
	if h.Jobs == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	cursor, _ := params.Cursor.Get()
	status, _ := params.Status.Get()
	kind, _ := params.Type.Get()
	queue, _ := params.Queue.Get()
	items, next, err := h.Jobs.List(ctx, jobs.ListInput{
		Cursor: string(cursor), Limit: params.Limit.Or(100), State: string(status), Kind: kind, Queue: queue,
	})
	if err != nil {
		if errors.Is(err, jobs.ErrInvalidCursor) {
			return nil, problem(http.StatusBadRequest, "invalid_cursor", "job cursor is invalid", err)
		}
		return nil, mapServiceError(err)
	}
	response := gen.JobPage{Tasks: make([]gen.Job, 0, len(items)), Meta: gen.JobPageMeta{}}
	for _, item := range items {
		response.Tasks = append(response.Tasks, jobResponse(item))
	}
	if next != "" {
		response.Meta.NextCursor = gen.NewOptCursor(gen.Cursor(next))
	}
	return &response, nil
}

func (h *Handler) CreateJob(ctx context.Context, req *gen.JobCreate) (gen.CreateJobRes, error) {
	if h.Jobs == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	queue, _ := req.Queue.Get()
	priority, _ := req.Priority.Get()
	maxAttempts, _ := req.MaxAttempts.Get()
	item, err := h.Jobs.Create(ctx, jobs.CreateInput{
		Kind: req.Type, Args: jsonRawMap(req.Args), Queue: queue,
		Priority: int(priority), MaxAttempts: int(maxAttempts), Tags: append([]string(nil), req.Tags...),
	})
	if err != nil {
		return nil, problem(http.StatusBadRequest, "invalid_job", "job could not be created", err)
	}
	response := jobResponse(item)
	return &response, nil
}

func (h *Handler) CreateUploadImport(ctx context.Context, req *gen.UploadImportRequest) (gen.CreateUploadImportRes, error) {
	if h.Jobs == nil || h.Catalog == nil || req == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if len(req.Sources) == 0 {
		return nil, problem(http.StatusUnprocessableEntity, "invalid_upload_import", "at least one upload source is required", nil)
	}
	args := jobs.UploadBatchArgs{
		UserID: userID, Destination: strings.TrimSpace(req.Destination), Exclude: append([]string(nil), req.Exclude...),
		PartConcurrency: int(req.PartConcurrency.Or(4)), ChunkSize: req.ChunkSize.Or(512 * 1024 * 1024), Encryption: req.Encryption.Or(false),
	}
	if args.Destination == "" {
		return nil, problem(http.StatusUnprocessableEntity, "invalid_upload_import", "destination must be a folder UUID or absolute drive path", nil)
	}
	if !strings.HasPrefix(args.Destination, "/") {
		if _, err := uuid.Parse(args.Destination); err != nil {
			return nil, problem(http.StatusUnprocessableEntity, "invalid_upload_import", "destination must be a folder UUID or absolute drive path", nil)
		}
	}
	if headers, ok := req.Headers.Get(); ok {
		args.Headers = cloneHeaders(headers)
	}
	if value, ok := req.MinSize.Get(); ok {
		args.MinSize = value
	}
	if value, ok := req.MaxSize.Get(); ok {
		args.MaxSize = value
	}
	args.Sources = make([]jobs.UploadSource, 0, len(req.Sources))
	for _, source := range req.Sources {
		item := jobs.UploadSource{Type: string(source.Type), Exclude: append([]string(nil), source.Exclude...)}
		if value, ok := source.Path.Get(); ok {
			item.Path = value
		}
		if value, ok := source.URL.Get(); ok {
			item.URL = value.String()
		}
		if value, ok := source.DestinationPath.Get(); ok {
			item.DestinationPath = value
		}
		if headers, ok := source.Headers.Get(); ok {
			item.Headers = cloneHeaders(headers)
		}
		if (item.Type == "local" && item.Path == "") || (item.Type == "http" && item.URL == "") {
			return nil, problem(http.StatusUnprocessableEntity, "invalid_upload_import", "source does not contain the required path or URL", nil)
		}
		args.Sources = append(args.Sources, item)
	}
	item, err := h.Jobs.InsertUploadBatch(ctx, args)
	if err != nil {
		return nil, mapServiceError(err)
	}
	response := jobResponse(item)
	return &response, nil
}

func cloneHeaders[T ~map[string]string](values T) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func (h *Handler) GetJobStatistics(ctx context.Context) (gen.GetJobStatisticsRes, error) {
	if h.Jobs == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	stats, err := h.Jobs.Statistics(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	return &gen.JobStatistics{
		Available: stats.Available, Cancelled: stats.Cancelled, Completed: stats.Completed,
		Discarded: stats.Discarded, Pending: stats.Pending, Retryable: stats.Retryable,
		Running: stats.Running, Scheduled: stats.Scheduled,
	}, nil
}

func (h *Handler) GetJob(ctx context.Context, params gen.GetJobParams) (gen.GetJobRes, error) {
	id, err := parseJobID(params.JobId)
	if err != nil {
		return nil, problem(http.StatusNotFound, "not_found", "job was not found", err)
	}
	item, err := h.Jobs.Get(ctx, id)
	if err != nil {
		return nil, mapJobError(err)
	}
	response := jobResponse(item)
	return &response, nil
}

func (h *Handler) CancelJob(ctx context.Context, params gen.CancelJobParams) (gen.CancelJobRes, error) {
	id, err := parseJobID(params.JobId)
	if err != nil {
		return nil, problem(http.StatusNotFound, "not_found", "job was not found", err)
	}
	item, err := h.Jobs.Cancel(ctx, id)
	if err != nil {
		return nil, mapJobError(err)
	}
	response := jobResponse(item)
	return &response, nil
}

func (h *Handler) RetryJob(ctx context.Context, params gen.RetryJobParams) (gen.RetryJobRes, error) {
	id, err := parseJobID(params.JobId)
	if err != nil {
		return nil, problem(http.StatusNotFound, "not_found", "job was not found", err)
	}
	item, err := h.Jobs.Retry(ctx, id)
	if err != nil {
		return nil, mapJobError(err)
	}
	response := jobResponse(item)
	return &response, nil
}

func (h *Handler) DeleteJob(ctx context.Context, params gen.DeleteJobParams) (gen.DeleteJobRes, error) {
	id, err := parseJobID(params.JobId)
	if err != nil {
		return nil, problem(http.StatusNotFound, "not_found", "job was not found", err)
	}
	item, err := h.Jobs.Get(ctx, id)
	if err != nil {
		return nil, mapJobError(err)
	}
	if isActiveJobState(item.State) {
		if _, err := h.Jobs.Cancel(ctx, id); err != nil {
			return nil, mapJobError(err)
		}
	} else if err := h.Jobs.Delete(ctx, id); err != nil {
		return nil, mapJobError(err)
	}
	return &gen.DeleteJobNoContent{}, nil
}

func (h *Handler) PurgeJobs(ctx context.Context, params gen.PurgeJobsParams) (gen.PurgeJobsRes, error) {
	count, err := h.Jobs.Purge(ctx, string(params.Status))
	if err != nil {
		if errors.Is(err, jobs.ErrInvalidJobState) {
			return nil, problem(http.StatusConflict, "invalid_state", "only finalized jobs can be purged", err)
		}
		return nil, mapServiceError(err)
	}
	return &gen.JobPurgeResult{Count: count}, nil
}

func (h *Handler) ListJobQueues(ctx context.Context) (gen.ListJobQueuesRes, error) {
	queues, err := h.Jobs.ListQueues(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	response := gen.JobQueueList{Queues: make([]gen.JobQueue, 0, len(queues))}
	for _, queue := range queues {
		response.Queues = append(response.Queues, gen.JobQueue{
			Name: queue.Name, Paused: queue.Paused, Available: queue.Available, Running: queue.Running,
			Retryable: queue.Retryable, Scheduled: queue.Scheduled,
		})
	}
	return &response, nil
}

func (h *Handler) PauseJobQueue(ctx context.Context, params gen.PauseJobQueueParams) (gen.PauseJobQueueRes, error) {
	if err := h.Jobs.PauseQueue(ctx, params.Queue); err != nil {
		return nil, mapJobError(err)
	}
	return &gen.PauseJobQueueNoContent{}, nil
}

func (h *Handler) ResumeJobQueue(ctx context.Context, params gen.ResumeJobQueueParams) (gen.ResumeJobQueueRes, error) {
	if err := h.Jobs.ResumeQueue(ctx, params.Queue); err != nil {
		return nil, mapJobError(err)
	}
	return &gen.ResumeJobQueueNoContent{}, nil
}

func parseJobID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, river.ErrNotFound
	}
	return id, nil
}

func isActiveJobState(state string) bool {
	switch state {
	case "available", "pending", "retryable", "running", "scheduled":
		return true
	default:
		return false
	}
}

func mapJobError(err error) error {
	if errors.Is(err, river.ErrNotFound) {
		return problem(http.StatusNotFound, "not_found", "job was not found", err)
	}
	if errors.Is(err, rivertype.ErrJobRunning) {
		return problem(http.StatusConflict, "job_running", "running jobs cannot be deleted", err)
	}
	return mapServiceError(err)
}

func jobResponse(item jobs.Job) gen.Job {
	response := gen.Job{
		ID: strconv.FormatInt(item.ID, 10), Status: gen.JobState(item.State), Type: item.Kind,
		Queue: item.Queue, Attempt: int32(item.Attempt), MaxAttempts: int32(item.MaxAttempts),
		Priority: int32(item.Priority), Tags: append([]string(nil), item.Tags...),
		Args:   rawMap[gen.JobArgs](item.Args),
		Errors: make([]gen.JobAttemptError, 0, len(item.Errors)), AttemptedBy: append([]string(nil), item.AttemptedBy...),
		CreatedAt: item.CreatedAt, ScheduledAt: item.ScheduledAt,
	}
	for _, attemptError := range item.Errors {
		errorResponse := gen.JobAttemptError{Attempt: int32(attemptError.Attempt), At: attemptError.At, Error: attemptError.Error}
		if attemptError.Trace != "" {
			errorResponse.Trace = gen.NewOptString(attemptError.Trace)
		}
		response.Errors = append(response.Errors, errorResponse)
	}
	if item.AttemptedAt != nil {
		response.StartedAt = gen.NewOptDateTime(*item.AttemptedAt)
	}
	if item.FinalizedAt != nil {
		response.CompletedAt = gen.NewOptDateTime(*item.FinalizedAt)
	}
	if value := rawString(item.Metadata, "parentId", "parent_id"); value != "" {
		response.ParentId = gen.NewOptString(value)
	}
	if value := rawString(item.Metadata, "description"); value != "" {
		response.Description = gen.NewOptString(value)
	}
	if value := rawString(item.Metadata, "message"); value != "" {
		response.Message = gen.NewOptString(value)
	} else if item.LastError != "" {
		response.Message = gen.NewOptString(item.LastError)
	}
	if len(item.Output) > 0 {
		var values map[string]json.RawMessage
		if json.Unmarshal(item.Output, &values) == nil {
			response.Output = gen.NewOptJobOutput(rawMap[gen.JobOutput](values))
		}
	}
	return response
}

func rawMap[T ~map[string]jx.Raw](input map[string]json.RawMessage) T {
	result := make(T, len(input))
	for key, value := range input {
		result[key] = jx.Raw(append([]byte(nil), value...))
	}
	return result
}

func rawString(values map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		var value string
		if raw, ok := values[key]; ok && json.Unmarshal(raw, &value) == nil {
			return value
		}
	}
	return ""
}
