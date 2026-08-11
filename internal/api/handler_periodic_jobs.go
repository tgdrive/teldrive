package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/divyam234/riverpro"
	"github.com/go-faster/jx"
	"github.com/riverqueue/river"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/jobs"
)

func (h *Handler) ListPeriodicJobs(ctx context.Context) (gen.ListPeriodicJobsRes, error) {
	items, err := h.Jobs.ListPeriodicJobs(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	response := gen.PeriodicJobList{Jobs: make([]gen.PeriodicJob, 0, len(items))}
	for _, item := range items {
		response.Jobs = append(response.Jobs, periodicJobResponse(item))
	}
	return &response, nil
}

func (h *Handler) GetPeriodicJobCatalog(context.Context) (gen.GetPeriodicJobCatalogRes, error) {
	templates := h.Jobs.PeriodicJobCatalog()
	response := gen.PeriodicJobCatalog{Templates: make([]gen.PeriodicJobTemplate, 0, len(templates))}
	for _, template := range templates {
		response.Templates = append(response.Templates, gen.PeriodicJobTemplate{
			Kind: template.Kind, Label: template.Label, Description: template.Description,
			DefaultId: template.ID, DefaultArgs: rawMap[gen.PeriodicJobTemplateDefaultArgs](template.DefaultArgs),
			DefaultQueue: template.DefaultQueue, RecommendedCron: template.DefaultCronExpression,
		})
	}
	return &response, nil
}

func (h *Handler) CreatePeriodicJob(ctx context.Context, req *gen.PeriodicJobCreate) (gen.CreatePeriodicJobRes, error) {
	paused, _ := req.Paused.Get()
	item, err := h.Jobs.CreatePeriodicJob(ctx, jobs.PeriodicJobInput{
		ID: req.ID, Kind: req.Kind, Args: jsonRawMap(req.Args), Queue: req.Queue,
		Priority: int(req.Priority), MaxAttempts: int(req.MaxAttempts), Tags: append([]string(nil), req.Tags...),
		Schedule: jobs.PeriodicSchedule{CronExpression: req.CronExpression, CronTimezone: req.CronTimezone},
		Paused:   paused,
	})
	if err != nil {
		return nil, mapPeriodicJobError(err)
	}
	response := periodicJobResponse(item)
	return &response, nil
}

func (h *Handler) UpdatePeriodicJob(ctx context.Context, req *gen.PeriodicJobUpdate, params gen.UpdatePeriodicJobParams) (gen.UpdatePeriodicJobRes, error) {
	paused, _ := req.Paused.Get()
	item, err := h.Jobs.UpdatePeriodicJob(ctx, params.PeriodicJobId, jobs.PeriodicJobInput{
		ID: params.PeriodicJobId, Kind: req.Kind, Args: jsonRawMap(req.Args), Queue: req.Queue,
		Priority: int(req.Priority), MaxAttempts: int(req.MaxAttempts), Tags: append([]string(nil), req.Tags...),
		Schedule: jobs.PeriodicSchedule{CronExpression: req.CronExpression, CronTimezone: req.CronTimezone},
		Paused:   paused,
	})
	if err != nil {
		return nil, mapPeriodicJobError(err)
	}
	response := periodicJobResponse(item)
	return &response, nil
}

func (h *Handler) DeletePeriodicJob(ctx context.Context, params gen.DeletePeriodicJobParams) (gen.DeletePeriodicJobRes, error) {
	if err := h.Jobs.DeletePeriodicJob(ctx, params.PeriodicJobId); err != nil {
		return nil, mapPeriodicJobError(err)
	}
	return &gen.DeletePeriodicJobNoContent{}, nil
}

func (h *Handler) PausePeriodicJob(ctx context.Context, params gen.PausePeriodicJobParams) (gen.PausePeriodicJobRes, error) {
	item, err := h.Jobs.PausePeriodicJob(ctx, params.PeriodicJobId)
	if err != nil {
		return nil, mapPeriodicJobError(err)
	}
	response := periodicJobResponse(item)
	return &response, nil
}

func (h *Handler) ResumePeriodicJob(ctx context.Context, params gen.ResumePeriodicJobParams) (gen.ResumePeriodicJobRes, error) {
	item, err := h.Jobs.ResumePeriodicJob(ctx, params.PeriodicJobId)
	if err != nil {
		return nil, mapPeriodicJobError(err)
	}
	response := periodicJobResponse(item)
	return &response, nil
}

func periodicJobResponse(item jobs.PeriodicJob) gen.PeriodicJob {
	response := gen.PeriodicJob{
		ID: item.ID, Kind: item.Kind, Args: rawMap[gen.PeriodicJobArgs](item.Args), Queue: item.Queue,
		Priority: int32(item.Priority), MaxAttempts: int32(item.MaxAttempts), Tags: append([]string(nil), item.Tags...),
		CronExpression: item.Schedule.CronExpression, CronTimezone: item.Schedule.CronTimezone,
		NextRunAt: item.NextRunAt, Paused: item.Paused, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
	if item.PausedAt != nil {
		response.PausedAt = gen.NewOptDateTime(*item.PausedAt)
	}
	return response
}

func jsonRawMap[T ~map[string]jx.Raw](input T) map[string]json.RawMessage {
	result := make(map[string]json.RawMessage, len(input))
	for key, value := range input {
		result[key] = json.RawMessage(append([]byte(nil), value...))
	}
	return result
}

func mapPeriodicJobError(err error) error {
	if errors.Is(err, riverpro.ErrPeriodicJobAlreadyExists) {
		return problem(http.StatusConflict, "already_exists", "periodic job already exists", err)
	}
	if errors.Is(err, river.ErrNotFound) {
		return problem(http.StatusNotFound, "not_found", "periodic job was not found", err)
	}
	return mapServiceError(err)
}
