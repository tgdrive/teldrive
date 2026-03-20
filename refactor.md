# TelDrive Refactor Plan

This document is a step-by-step refactor plan for the TelDrive backend.

It is written for execution by simple coding agents. Do not improvise the architecture. Follow the phases in order. Keep changes small. After each phase, run the listed verification steps before moving on.

## Goal

Refactor the current service layer into small, feature-based application services with narrow dependencies so that:

- each use case has a single responsibility;
- tests become easier to write and maintain;
- feature logic is no longer concentrated in one large `apiService` type;
- infrastructure details stay behind small interfaces;
- future agents can work on one feature without understanding the whole app.

## Current Problems

These are the main hotspots that justify the refactor:

- `pkg/services/api.go` defines one large `apiService` with many unrelated responsibilities.
- `pkg/services/file.go` mixes validation, orchestration, database writes, cache invalidation, event publishing, and Telegram operations.
- `pkg/services/periodic_jobs.go` contains large branching logic and registry orchestration in one file.
- `pkg/services/telegram.go` owns too many unrelated Telegram concerns in one abstraction.
- `pkg/repositories/interfaces.go` exposes a large repository bag, which makes every service depend on more than it needs.
- integration tests in `tests/integration/` currently exercise broad slices of behavior because unit-test seams are weak.

## Target Architecture

Use a pragmatic feature-based application architecture.

### Design Rules

- Keep `cmd/` as the composition root only.
- Keep transport handlers thin.
- Keep business logic in feature packages.
- Define interfaces in the consumer package, not the producer package, when practical.
- Inject narrow dependencies into each feature service.
- Prefer concrete structs internally.
- Keep functions synchronous unless async behavior is required by the product.
- Keep `context.Context` as the first argument in all request-scoped operations.
- Return errors upward; do not log and return the same error in the same layer.

### Target Package Shape

Create feature-focused packages under `internal/app/`.

Planned target layout:

```text
internal/app/
  auth/
    commands.go
    queries.go
    ports.go
    service.go
  files/
    commands.go
    queries.go
    ports.go
    service.go
    events.go
  jobs/
    commands.go
    queries.go
    ports.go
    service.go
  periodicjobs/
    commands.go
    queries.go
    ports.go
    service.go
    registry.go
    handlers.go
  shares/
    commands.go
    queries.go
    ports.go
    service.go
  uploads/
    commands.go
    queries.go
    ports.go
    service.go
  users/
    commands.go
    queries.go
    ports.go
    service.go
```

The existing `pkg/services/` package should become a thin API adapter layer that delegates to `internal/app/*` services.

## Refactor Constraints

- Do not change public API behavior unless a phase explicitly says to.
- Do not combine multiple feature migrations in one commit-sized change.
- Do not rewrite repository implementations and feature services in the same phase unless necessary for a clean seam.
- Do not introduce a global service locator or container object.
- Do not create interfaces only for mocking when a concrete type is enough.
- Do not move all code at once. Use strangler-style migration.
- Keep integration tests green throughout the process.

## Testing Strategy

Testing is a first-class goal of this refactor.

### Desired Test Pyramid

- unit tests for application services with hand-written fakes;
- small adapter tests for repository wrappers and transport mapping;
- existing integration tests kept as regression coverage.

### Testing Rules

- Every extracted feature service should gain focused unit tests before large deletions of old logic.
- Existing integration tests must continue to pass.
- When moving logic from `pkg/services/` into `internal/app/`, add unit tests around the moved logic rather than expanding integration fixtures.
- Prefer table-driven tests for validation and mapping logic.
- Prefer hand-written fakes over large mocking frameworks unless the repo already standardizes on one.

### Standard Verification Commands

Run these after each meaningful phase:

```bash
go test ./...
task lint
```

If a phase only touches one feature and full validation is too slow during intermediate work, run these first and then run the full suite before phase completion:

```bash
go test ./tests/integration/...
go test ./pkg/services/...
go test ./internal/...
```

## Phase Plan

Execute phases in order.

---

## Phase 0 - Stabilize The Starting Point

### Objective

Create a safe baseline before changing architecture.

### Steps

1. Record the current package responsibilities in brief comments inside this file if new discoveries appear.
2. Confirm the API server wiring path in `cmd/run.go`.
3. Confirm where `apiService` is created in tests and runtime.
4. Run the baseline test suite.
5. Do not change architecture in this phase.

### Deliverables

- passing baseline tests;
- no production code changes except harmless test helpers or documentation.

### Acceptance Criteria

- the repo builds and tests before refactor begins;
- agents know the current composition root and test server setup.

---

## Phase 1 - Introduce Shared Application Conventions

### Objective

Create the minimum scaffolding needed for feature services.

### Steps

1. Create `internal/app/` and feature directories without moving logic yet.
2. Add `ports.go` files only where a phase immediately needs them.
3. Add small request/response structs only when they improve clarity.
4. Define naming conventions:
   - command methods mutate state;
   - query methods read state only;
   - service constructors accept only direct dependencies.
5. Keep all old runtime behavior unchanged.

### Deliverables

- empty or near-empty feature package skeletons;
- agreed naming conventions for future phases.

### Acceptance Criteria

- no behavior changes;
- compile remains green.

---

## Phase 2 - Extract File Queries First

### Why First

File behavior is the largest SRP problem and the highest-value area for testing improvement.

### Scope

Move read-only file operations out of `pkg/services/file.go` first because they are lower risk than write flows.

Target methods:

- `FilesCategoryStats`
- `FilesGetById`
- `FilesList`
- `FilesChildren`
- `FilesListShares`

### New Package

`internal/app/files`

### New Dependencies To Define

Define small consumer-owned interfaces in `internal/app/files/ports.go`.

Suggested starting interfaces:

```go
type FileReader interface {
    GetByID(ctx context.Context, id uuid.UUID) (*model.Files, error)
    List(ctx context.Context, params repositories.FileQueryParams) ([]model.Files, error)
    GetFullPath(ctx context.Context, fileID uuid.UUID) (string, error)
    CategoryStats(ctx context.Context, userID int64) ([]repositories.CategoryStats, error)
}

type ShareReader interface {
    GetByFileID(ctx context.Context, fileID uuid.UUID) ([]model.FileShares, error)
}
```

Do not copy this blindly if a smaller interface is enough.

### Steps

1. Create `internal/app/files/queries.go`.
2. Move query logic into a `QueryService` or similar concrete type.
3. Keep transport-level API mapping in `pkg/services/file.go` initially if that reduces churn.
4. Make `pkg/services/file.go` call the new query service.
5. Add unit tests for file query behavior.
6. Keep repository calls unchanged for now.

### Tests To Add

- list parameter mapping;
- next cursor behavior;
- category stats mapping;
- file-not-found translation path;
- share list mapping.

### Acceptance Criteria

- read-only file endpoints still pass integration tests;
- new file query unit tests exist;
- `pkg/services/file.go` becomes smaller.

---

## Phase 3 - Extract File Commands

### Scope

Move state-changing file operations into `internal/app/files/commands.go`.

Target methods:

- `FilesCreate`
- `FilesUpdate`
- `FilesDeleteById`
- `FilesDelete`
- `FilesRestore`
- `FilesMove`
- `FilesMkdir`
- `FilesCopy`
- `FilesCreateShare`
- `FilesEditShare`
- `FilesDeleteShare`

### Required Dependency Split

Split responsibilities into narrow interfaces. Suggested groups:

- file store;
- upload store;
- share store;
- cache invalidator;
- event recorder;
- channel resolver;
- telegram file transfer client;
- transaction runner.

### Key Rule

Do not inject `*repositories.Repositories` directly into the new command service.

Instead, define only the methods actually used.

### Steps

1. Create command-specific interfaces in `internal/app/files/ports.go`.
2. Move pure helper functions first, such as parts mapping and JSON handling, if useful.
3. Extract validation into small functions where possible.
4. Move create/update/delete/move/copy logic into a concrete command service.
5. Keep `pkg/services/file.go` as an adapter that:
   - reads auth/user context;
   - maps `api.*` types to command inputs;
   - delegates to the command service;
   - maps results back to `api.*`.
6. Add unit tests for each major command path.
7. Keep transaction orchestration behavior identical.

### Tests To Add

- create file with direct parts;
- create file from upload ID;
- update file content and hash behavior;
- move single file with rename;
- bulk move;
- delete with cache invalidation;
- restore trashed file;
- copy file using telegram collaborator;
- share create/edit/delete behavior.

### Acceptance Criteria

- file mutation flows remain green in integration tests;
- file command service has direct unit tests using small fakes;
- `pkg/services/file.go` contains mostly mapping and delegation.

---

## Phase 4 - Split Telegram Responsibilities

### Objective

Reduce the scope of `pkg/services/telegram.go` so features depend on smaller capabilities.

### Problem

The current `TelegramService` is too broad. Many callers need only a small subset.

### Target Capability Groups

- auth/session capability;
- profile/account capability;
- channel capability;
- media retrieval capability;
- upload capability;
- file copy capability;
- bot selection capability.

### Steps

1. Identify which feature uses which Telegram methods.
2. Create small interfaces close to the consuming feature packages.
3. Do not immediately split the concrete implementation file unless needed.
4. First narrow dependencies at the call sites.
5. Only after that, consider splitting `pkg/services/telegram.go` into multiple files.

### Acceptance Criteria

- no new feature package depends on the full `TelegramService` interface unless it truly needs most methods;
- file and upload services depend on smaller Telegram-facing interfaces.

---

## Phase 5 - Extract Jobs Feature

### Scope

Move logic from `pkg/services/jobs.go` into `internal/app/jobs`.

### Target Behavior

- list jobs;
- insert allowed jobs;
- get job by ID;
- cancel job;
- delete job;
- ownership checks.

### Steps

1. Create `internal/app/jobs/service.go`, `commands.go`, `queries.go`, and `ports.go`.
2. Move job list parameter creation into query logic.
3. Move job ownership verification into a reusable helper in the jobs package.
4. Keep API type mapping in the adapter layer if that reduces churn.
5. Add unit tests for cursor parsing and ownership checks.

### Acceptance Criteria

- jobs API behavior remains unchanged;
- `pkg/services/jobs.go` is reduced to delegation and mapping.

---

## Phase 6 - Extract Periodic Jobs Feature

### Why This Is Separate

`pkg/services/periodic_jobs.go` is large and has multiple concerns:

- API-facing CRUD;
- default preset creation;
- args normalization;
- cron validation;
- registry synchronization;
- runtime job insertion;
- kind-specific branching.

### Target Package

`internal/app/periodicjobs`

### Target Substructure

```text
internal/app/periodicjobs/
  service.go
  commands.go
  queries.go
  ports.go
  registry.go
  handlers.go
```

### Key Design Change

Replace large `switch` blocks on job kind with a handler registry.

Suggested shape:

```go
type KindHandler interface {
    Kind() string
    NormalizeArgs(args repositories.PeriodicJobArgs) (repositories.PeriodicJobArgs, error)
    ValidateCreate(req api.PeriodicJobCreate) error
    ValidateUpdate(row JobRow, req api.PeriodicJobUpdate) error
    BuildRuntimeInsert(row JobRow) (river.JobArgs, *river.InsertOpts, error)
}
```

Adjust types as needed. Keep the concept even if the exact signature changes.

### Handlers To Create

- sync run handler;
- clean old events handler;
- clean stale uploads handler;
- clean pending files handler.

### Steps

1. Move cron validation and shared helpers into the periodic jobs package.
2. Introduce handler registry and migrate one kind first.
3. Once one kind works, migrate the remaining kinds.
4. Move default preset logic into a dedicated file.
5. Move registry synchronization into `registry.go`.
6. Move runtime insert logic out of the API adapter.
7. Add unit tests per handler.

### Tests To Add

- cron validation;
- preset normalization;
- maintenance retention validation;
- enable/disable registry behavior;
- runtime insert mapping for each kind;
- kind-specific update restrictions.

### Acceptance Criteria

- no large kind switch remains in the adapter layer;
- each periodic job kind has isolated tests;
- registry behavior remains correct in integration tests.

---

## Phase 7 - Extract Uploads, Shares, Auth, Users

### Objective

Finish the remaining feature packages after the patterns are proven in Files and PeriodicJobs.

### Order

Use this order:

1. uploads
2. shares
3. auth
4. users

This order keeps storage-heavy and file-adjacent behavior together first.

### Per-Feature Steps

For each feature:

1. identify query methods;
2. identify command methods;
3. define narrow ports;
4. move logic into `internal/app/<feature>`;
5. keep `pkg/services/*.go` as thin adapter;
6. add unit tests;
7. run integration tests.

### Acceptance Criteria

- each feature owns its own application logic;
- transport layer no longer contains business orchestration.

---

## Phase 8 - Shrink `pkg/services` To Adapter Layer Only

### Objective

Make `pkg/services` a thin compatibility layer, or replace it entirely if safe.

### Desired End State

Files in `pkg/services/` should mostly do these things:

- satisfy generated API interfaces;
- read auth context;
- call application services;
- translate errors to API errors;
- map API DTOs to internal command/query structs.

They should not own complex orchestration logic.

### Steps

1. Review each remaining file in `pkg/services/`.
2. Remove business logic that was left behind during earlier phases.
3. Keep shared error translation in one place if helpful.
4. Keep raw transport helper logic separate from app services.

### Acceptance Criteria

- `pkg/services/api.go` no longer acts as the main place for business logic;
- feature services hold the behavior.

---

## Phase 9 - Revisit Repository Boundaries

### Objective

Only after feature services are extracted, revisit repository abstractions.

### Important Rule

Do not start here. This is intentionally late.

### Problem

`pkg/repositories/interfaces.go` currently mixes many repository contracts in one place.

### Target Improvement

- keep concrete repository implementations where they are if stable;
- gradually move consumer-owned interfaces closer to the feature packages;
- optionally keep shared storage implementations in `pkg/repositories`.

### Steps

1. Audit which repository interfaces are still truly shared.
2. Move only consumer-specific interfaces into feature packages.
3. Keep concrete Jet/GORM repository types unchanged unless there is a real need to split them.
4. Avoid a large repository rewrite.

### Acceptance Criteria

- feature packages no longer depend on the whole repository bag;
- repository abstractions are smaller and purpose-driven.

---

## Phase 10 - Final Cleanup And Documentation

### Objective

Leave the codebase easy for future agents and humans to navigate.

### Steps

1. Remove dead helpers and duplicate logic.
2. Ensure package names are simple and intentional.
3. Add package docs where helpful.
4. Update any contributor docs if the new architecture is now standard.
5. Add a short architecture overview to the repo if needed.

### Acceptance Criteria

- no dead transition code remains;
- the final structure matches the implemented architecture.

## Required Agent Workflow Per Phase

Any agent executing this plan must follow this checklist.

### Before Coding

1. Read the target files fully.
2. Identify the exact methods in scope for the current phase.
3. Confirm the existing tests covering those methods.
4. Do not expand the phase unless blocked.

### During Coding

1. Move logic in small slices.
2. Keep old call paths alive until the new path is verified.
3. Add or update tests with each extraction.
4. Do not mix unrelated cleanups with architecture moves.

### Before Finishing The Phase

1. Run feature-local tests.
2. Run full integration tests.
3. Run `go test ./...`.
4. Run `task lint`.
5. Update this file if the plan changed.

## Definition Of Done

The refactor is done only when all of the following are true:

- no major business workflow is implemented directly inside the large adapter files in `pkg/services/`;
- feature services exist under `internal/app/`;
- each feature service uses narrow dependencies;
- unit tests cover application behavior without broad integration setup;
- integration tests still pass;
- transport layer remains behavior-compatible with the existing API.

## Anti-Patterns To Reject During Review

Reject any change that introduces one of these patterns:

- another giant service with many unrelated methods;
- passing `*repositories.Repositories` into every new service;
- defining interfaces on the implementation side solely for mocks;
- combining transport DTOs and core business structs in the same layer without need;
- logging and returning the same error from the same layer;
- hidden goroutines for normal request flow;
- global mutable test hooks.

## Suggested Commit Sequence

If doing this over multiple commits, prefer this order:

1. scaffold `internal/app/`
2. extract file queries
3. extract file commands
4. narrow Telegram dependencies
5. extract jobs
6. extract periodic jobs registry and handlers
7. extract uploads
8. extract shares
9. extract auth
10. extract users
11. thin `pkg/services`
12. cleanup and docs

## Final Note For Future Agents

This plan is intentionally conservative.

Do not attempt a big-bang rewrite.

If a phase reveals an unexpected dependency knot, stop and create a smaller intermediate seam first. The correct move is almost always:

1. narrow the dependency;
2. move one behavior slice;
3. add tests;
4. then delete the old path.
