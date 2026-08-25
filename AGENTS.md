# Repository Guide

## Sources Of Truth

- Use `justfile` for supported workflows; it loads a root `.env` automatically.
- TypeSpec files in `typespec/` own the HTTP contract. Do not hand-edit `openapi/teldrive.openapi.yaml`, `internal/api/gen/`, or `ui/src/api/schema.ts`; run `just generate-api`, `just generate-ui`, or `just generate` as appropriate.
- SQL lives in `db/queries/` and migrations in `db/migrations/`. Do not edit `internal/db/sqlcgen/` manually.
- Always run `just generate-db` after SQL changes. A bare `sqlc generate` omits the required `go run ./internal/tools/patchsqlc` step and breaks configurable PostgreSQL schema rewriting in `internal/db/sqlcgen/db.go`.
- Preserve `/* TEMPLATE: schema */` markers in SQL; the generated DB wrapper replaces them with the configured schema at runtime.

## Commands

- Install pinned JS dependencies: `just install-tools` (uses Bun for both `typespec/` and `ui/`).
- Backend unit tests: `go test ./...`; focused package/test: `go test ./internal/transfer -run '^TestName$'`.
- Integration tests require Podman and must use the harness: `scripts/test-postgres.sh go test -tags=integration ./internal/uploads`; use `just test-integration` for all packages.
- Race tests also require the PostgreSQL harness: `just test-race`.
- Full project validation: `just check`. This regenerates artifacts and runs lint, UI checks/build, unit tests, and the Podman-backed 80% core coverage gate; it is intentionally expensive.
- UI checks: `just ui-check`. Browser E2E with real backend/filesystem Telegram: `just ui-e2e`; `scripts/test-ui.sh start|test|status|stop` supports a reusable environment.
- Format only handwritten code with `just format`; generated Go directories are deliberately excluded.

## Architecture

- `cmd/teldrive` is the CLI entrypoint; `internal/app/app.go` is the composition root and owns migrations, services, HTTP routing, workers, and shutdown order.
- `internal/api` adapts the ogen contract; domain behavior belongs in packages such as `catalog`, `uploads`, `transfer`, `fileops`, and `shares`, not generated handlers.
- `internal/telegramstore` is the external storage boundary. Production uses gotd; integration and UI tests can use the filesystem backend.
- `db/migrations` includes application schema; startup also runs River/RiverPro migrations before opening the long-lived pool.
- The UI is a Vite/React app in `ui/`; `ui/ui.go` embeds `ui/dist`, so `just build` builds the UI before compiling the server binary.

## Generated Changes

- API generation intentionally fails if ogen emits an unimplemented-handler fallback or operation counts diverge; implement every generated handler method explicitly.
- Review generated diffs after `just generate`; generation can touch Go API code, SQL query code, OpenAPI, and the UI schema together.
