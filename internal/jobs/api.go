package jobs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver"
	"github.com/riverqueue/river/rivertype"
)

var ErrInvalidCursor = errors.New("invalid job cursor")

type JobError struct {
	Attempt int
	At      time.Time
	Error   string
	Trace   string
}

type Job struct {
	ID          int64
	State       string
	Attempt     int
	MaxAttempts int
	AttemptedAt *time.Time
	CreatedAt   time.Time
	FinalizedAt *time.Time
	ScheduledAt time.Time
	Priority    int
	Kind        string
	Queue       string
	Args        map[string]json.RawMessage
	Metadata    map[string]json.RawMessage
	Output      json.RawMessage
	Errors      []JobError
	AttemptedBy []string
	Tags        []string
	LastError   string
}

type ListInput struct {
	Cursor string
	Limit  int32
	State  string
	Kind   string
	Queue  string
}

type Statistics struct {
	Available int64
	Cancelled int64
	Completed int64
	Discarded int64
	Pending   int64
	Retryable int64
	Running   int64
	Scheduled int64
}

func (r *Runtime) List(ctx context.Context, input ListInput) ([]Job, string, error) {
	if r == nil || r.client == nil {
		return nil, "", ErrRuntimeNotConfigured
	}
	if input.Limit <= 0 || input.Limit > 200 {
		input.Limit = 100
	}
	beforeID, err := decodeCursor(input.Cursor)
	if err != nil {
		return nil, "", err
	}

	params := river.NewJobListParams().
		OrderBy(river.JobListOrderByID, river.SortOrderDesc).
		First(int(input.Limit) + 1)
	if beforeID > 0 {
		params = params.Where("id < @before_id", river.NamedArgs{"before_id": beforeID})
	}
	if input.State != "" {
		params = params.States(rivertype.JobState(input.State))
	}
	if input.Kind != "" {
		params = params.Kinds(input.Kind)
	}
	if input.Queue != "" {
		params = params.Queues(input.Queue)
	}

	result, err := r.client.JobList(ctx, params)
	if err != nil {
		return nil, "", fmt.Errorf("list jobs: %w", err)
	}
	items := make([]Job, 0, len(result.Jobs))
	for _, row := range result.Jobs {
		items = append(items, jobFromRiver(row))
	}
	var next string
	if len(items) > int(input.Limit) {
		items = items[:input.Limit]
		next = encodeCursor(items[len(items)-1].ID)
	}
	return items, next, nil
}

func (r *Runtime) Statistics(ctx context.Context) (Statistics, error) {
	if r == nil || r.client == nil {
		return Statistics{}, ErrRuntimeNotConfigured
	}
	counts, err := r.client.Driver().GetExecutor().JobCountByAllStates(ctx, &riverdriver.JobCountByAllStatesParams{Schema: r.schema})
	if err != nil {
		return Statistics{}, fmt.Errorf("job statistics: %w", err)
	}
	return Statistics{
		Available: int64(counts[rivertype.JobStateAvailable]),
		Cancelled: int64(counts[rivertype.JobStateCancelled]),
		Completed: int64(counts[rivertype.JobStateCompleted]),
		Discarded: int64(counts[rivertype.JobStateDiscarded]),
		Pending:   int64(counts[rivertype.JobStatePending]),
		Retryable: int64(counts[rivertype.JobStateRetryable]),
		Running:   int64(counts[rivertype.JobStateRunning]),
		Scheduled: int64(counts[rivertype.JobStateScheduled]),
	}, nil
}

func (r *Runtime) Cancel(ctx context.Context, id int64) (Job, error) {
	if r == nil || r.client == nil {
		return Job{}, ErrRuntimeNotConfigured
	}
	row, err := r.client.JobCancel(ctx, id)
	if err != nil {
		return Job{}, err
	}
	return jobFromRiver(row), nil
}

func (r *Runtime) Retry(ctx context.Context, id int64) (Job, error) {
	if r == nil || r.client == nil {
		return Job{}, ErrRuntimeNotConfigured
	}
	row, err := r.client.JobRetry(ctx, id)
	if err != nil {
		return Job{}, err
	}
	return jobFromRiver(row), nil
}

func (r *Runtime) Delete(ctx context.Context, id int64) error {
	if r == nil || r.client == nil {
		return ErrRuntimeNotConfigured
	}
	_, err := r.client.JobDelete(ctx, id)
	return err
}

func jobFromRiver(row *rivertype.JobRow) Job {
	item := Job{
		ID: row.ID, State: string(row.State), Attempt: row.Attempt, MaxAttempts: row.MaxAttempts,
		AttemptedAt: row.AttemptedAt, CreatedAt: row.CreatedAt, FinalizedAt: row.FinalizedAt,
		ScheduledAt: row.ScheduledAt, Priority: row.Priority, Kind: row.Kind, Queue: row.Queue,
		Args: map[string]json.RawMessage{}, Metadata: map[string]json.RawMessage{},
		AttemptedBy: append([]string(nil), row.AttemptedBy...), Tags: append([]string(nil), row.Tags...),
	}
	_ = json.Unmarshal(row.EncodedArgs, &item.Args)
	_ = json.Unmarshal(row.Metadata, &item.Metadata)
	item.Output = append(json.RawMessage(nil), row.Output()...)
	item.Errors = make([]JobError, 0, len(row.Errors))
	for _, attemptError := range row.Errors {
		item.Errors = append(item.Errors, JobError{
			Attempt: attemptError.Attempt,
			At:      attemptError.At,
			Error:   attemptError.Error,
			Trace:   attemptError.Trace,
		})
	}
	if len(item.Errors) > 0 {
		item.LastError = item.Errors[len(item.Errors)-1].Error
	}
	return item
}

func encodeCursor(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(id, 10)))
}
func decodeCursor(cursor string) (int64, error) {
	if strings.TrimSpace(cursor) == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, ErrInvalidCursor
	}
	id, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, ErrInvalidCursor
	}
	return id, nil
}
