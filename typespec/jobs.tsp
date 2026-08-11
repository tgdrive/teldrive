using TypeSpec.Http;
using TypeSpec.OpenAPI;

namespace TelDrive;

enum JobState {
  available: "available",
  cancelled: "cancelled",
  completed: "completed",
  discarded: "discarded",
  pending: "pending",
  retryable: "retryable",
  running: "running",
  scheduled: "scheduled",
}

model JobAttemptError {
  attempt: int32;
  at: utcDateTime;
  error: string;
  trace?: string;
}

model Job {
  id: string;
  status: JobState;
  type: string;
  parentId?: string;
  queue: string;
  description?: string;
  message?: string;
  attempt: int32;
  maxAttempts: int32;
  priority: int32;
  tags: string[];
  args: Record<unknown>;
  output?: Record<unknown>;
  errors: JobAttemptError[];
  attemptedBy: string[];
  createdAt: utcDateTime;
  scheduledAt: utcDateTime;
  startedAt?: utcDateTime;
  completedAt?: utcDateTime;
}

model JobPageMeta {
  nextCursor?: Cursor;
}

model JobPage {
  tasks: Job[];
  meta: JobPageMeta;
}

model JobListQuery is CursorQuery {
  @query
  status?: JobState;

  @query
  type?: string;

  @query
  queue?: string;
}

model JobStatistics {
  available: int64;
  cancelled: int64;
  completed: int64;
  discarded: int64;
  pending: int64;
  retryable: int64;
  running: int64;
  scheduled: int64;
}

model JobPurgeQuery {
  @query
  status: JobState;
}

model JobPurgeResult {
  count: int64;
}

model JobCreate {
  type: string;
  args: Record<unknown>;
  queue?: string;
  priority?: int32;
  maxAttempts?: int32;
  tags?: string[];
}

model JobQueue {
  name: string;
  paused: boolean;
  available: int64;
  running: int64;
  retryable: int64;
  scheduled: int64;
}

model JobQueueList {
  queues: JobQueue[];
}

model PeriodicJob {
  id: string;
  kind: string;
  args: Record<unknown>;
  queue: string;
  priority: int32;
  maxAttempts: int32;
  tags: string[];
  cronExpression: string;
  cronTimezone: string;
  nextRunAt: utcDateTime;
  paused: boolean;
  pausedAt?: utcDateTime;
  createdAt: utcDateTime;
  updatedAt: utcDateTime;
}

model PeriodicJobList {
  jobs: PeriodicJob[];
}

model PeriodicJobTemplate {
  kind: string;
  label: string;
  description: string;
  defaultId: string;
  defaultArgs: Record<unknown>;
  defaultQueue: string;
  recommendedCron: string;
}

model PeriodicJobCatalog {
  templates: PeriodicJobTemplate[];
}

model PeriodicJobCreate {
  id: string;
  kind: string;
  args: Record<unknown>;
  queue: string;
  priority: int32;
  maxAttempts: int32;
  tags: string[];
  cronExpression: string;
  cronTimezone: string;
  paused?: boolean;
}

model PeriodicJobUpdate {
  kind: string;
  args: Record<unknown>;
  queue: string;
  priority: int32;
  maxAttempts: int32;
  tags: string[];
  cronExpression: string;
  cronTimezone: string;
  paused?: boolean;
}

@tag("Jobs")
@route("/v1/jobs")
@useAuth(TelDriveAuth)
namespace Jobs {
  @get
  @operationId("listJobs")
  op list(...JobListQuery): JsonResponse<200, JobPage> | ErrorResponse<400> | ErrorResponse<401>;

  @post
  @operationId("createJob")
  op create(@body body: JobCreate): JsonResponse<201, Job> | ErrorResponse<400> | ErrorResponse<401> | ErrorResponse<409>;

  @get
  @route("/statistics")
  @operationId("getJobStatistics")
  op statistics(): JsonResponse<200, JobStatistics> | ErrorResponse<401>;

  @get
  @route("/queues")
  @operationId("listJobQueues")
  op queues(): JsonResponse<200, JobQueueList> | ErrorResponse<401>;

  @post
  @route("/queues/{queue}/pause")
  @operationId("pauseJobQueue")
  op pauseQueue(@path queue: string): EmptyResponse<204> | ErrorResponse<401> | ErrorResponse<404>;

  @post
  @route("/queues/{queue}/resume")
  @operationId("resumeJobQueue")
  op resumeQueue(@path queue: string): EmptyResponse<204> | ErrorResponse<401> | ErrorResponse<404>;

  @delete
  @route("/purge")
  @operationId("purgeJobs")
  op purge(...JobPurgeQuery): JsonResponse<200, JobPurgeResult> | ErrorResponse<401> | ErrorResponse<409>;

  @get
  @route("/{jobId}")
  @operationId("getJob")
  op get(@path jobId: string): JsonResponse<200, Job> | ErrorResponse<401> | ErrorResponse<404>;

  @post
  @route("/{jobId}/cancel")
  @operationId("cancelJob")
  op cancel(@path jobId: string): JsonResponse<200, Job> | ErrorResponse<401> | ErrorResponse<404> | ErrorResponse<409>;

  @post
  @route("/{jobId}/retry")
  @operationId("retryJob")
  op retry(@path jobId: string): JsonResponse<200, Job> | ErrorResponse<401> | ErrorResponse<404> | ErrorResponse<409>;

  @delete
  @route("/{jobId}")
  @operationId("deleteJob")
  op delete(@path jobId: string): EmptyResponse<204> | ErrorResponse<401> | ErrorResponse<404> | ErrorResponse<409>;
}

@tag("Periodic Jobs")
@route("/v1/periodic-jobs")
@useAuth(TelDriveAuth)
namespace PeriodicJobs {
  @get
  @operationId("listPeriodicJobs")
  op list(): JsonResponse<200, PeriodicJobList> | ErrorResponse<401>;

  @post
  @operationId("createPeriodicJob")
  op create(@body body: PeriodicJobCreate): JsonResponse<201, PeriodicJob> | ErrorResponse<400> | ErrorResponse<401> | ErrorResponse<409>;

  @get
  @route("/catalog")
  @operationId("getPeriodicJobCatalog")
  op catalog(): JsonResponse<200, PeriodicJobCatalog> | ErrorResponse<401>;

  @put
  @route("/{periodicJobId}")
  @operationId("updatePeriodicJob")
  op update(@path periodicJobId: string, @body body: PeriodicJobUpdate): JsonResponse<200, PeriodicJob> | ErrorResponse<400> | ErrorResponse<401> | ErrorResponse<404>;

  @delete
  @route("/{periodicJobId}")
  @operationId("deletePeriodicJob")
  op delete(@path periodicJobId: string): EmptyResponse<204> | ErrorResponse<401> | ErrorResponse<404>;

  @post
  @route("/{periodicJobId}/pause")
  @operationId("pausePeriodicJob")
  op pause(@path periodicJobId: string): JsonResponse<200, PeriodicJob> | ErrorResponse<401> | ErrorResponse<404>;

  @post
  @route("/{periodicJobId}/resume")
  @operationId("resumePeriodicJob")
  op resume(@path periodicJobId: string): JsonResponse<200, PeriodicJob> | ErrorResponse<401> | ErrorResponse<404>;
}
