package services

import (
	"context"
	"errors"
	"sync"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

var errRuntimeDependencyNotReady = errors.New("runtime dependency is not configured")

type JobClientRef struct {
	mu     sync.RWMutex
	target jobClient
}

func NewJobClientRef() *JobClientRef {
	return &JobClientRef{}
}

func (r *JobClientRef) Set(target jobClient) {
	r.mu.Lock()
	r.target = target
	r.mu.Unlock()
}

func (r *JobClientRef) client() (jobClient, error) {
	r.mu.RLock()
	target := r.target
	r.mu.RUnlock()
	if target == nil {
		return nil, errRuntimeDependencyNotReady
	}
	return target, nil
}

func (r *JobClientRef) Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	client, err := r.client()
	if err != nil {
		return nil, err
	}
	return client.Insert(ctx, args, opts)
}

func (r *JobClientRef) JobList(ctx context.Context, params *river.JobListParams) (*river.JobListResult, error) {
	client, err := r.client()
	if err != nil {
		return nil, err
	}
	return client.JobList(ctx, params)
}

func (r *JobClientRef) JobGet(ctx context.Context, id int64) (*rivertype.JobRow, error) {
	client, err := r.client()
	if err != nil {
		return nil, err
	}
	return client.JobGet(ctx, id)
}

func (r *JobClientRef) JobUpdate(ctx context.Context, id int64, params *river.JobUpdateParams) (*rivertype.JobRow, error) {
	client, err := r.client()
	if err != nil {
		return nil, err
	}
	return client.JobUpdate(ctx, id, params)
}

func (r *JobClientRef) JobCancel(ctx context.Context, jobID int64) (*rivertype.JobRow, error) {
	client, err := r.client()
	if err != nil {
		return nil, err
	}
	return client.JobCancel(ctx, jobID)
}

func (r *JobClientRef) JobDelete(ctx context.Context, id int64) (*rivertype.JobRow, error) {
	client, err := r.client()
	if err != nil {
		return nil, err
	}
	return client.JobDelete(ctx, id)
}

type PeriodicJobRegistryRef struct {
	mu     sync.RWMutex
	target periodicJobRegistry
}

func NewPeriodicJobRegistryRef() *PeriodicJobRegistryRef {
	return &PeriodicJobRegistryRef{}
}

func (r *PeriodicJobRegistryRef) Set(target periodicJobRegistry) {
	r.mu.Lock()
	r.target = target
	r.mu.Unlock()
}

func (r *PeriodicJobRegistryRef) registry() (periodicJobRegistry, bool) {
	r.mu.RLock()
	target := r.target
	r.mu.RUnlock()
	return target, target != nil
}

func (r *PeriodicJobRegistryRef) AddMany(periodicJobs []*river.PeriodicJob) []rivertype.PeriodicJobHandle {
	registry, ok := r.registry()
	if !ok {
		return nil
	}
	return registry.AddMany(periodicJobs)
}

func (r *PeriodicJobRegistryRef) AddSafely(periodicJob *river.PeriodicJob) (rivertype.PeriodicJobHandle, error) {
	registry, ok := r.registry()
	if !ok {
		return 0, errRuntimeDependencyNotReady
	}
	return registry.AddSafely(periodicJob)
}

func (r *PeriodicJobRegistryRef) RemoveByID(id string) bool {
	registry, ok := r.registry()
	if !ok {
		return false
	}
	return registry.RemoveByID(id)
}
