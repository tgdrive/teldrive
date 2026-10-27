# Repository Guide

## Required Environment

- Run every project command through `direnv exec . <command>` from the repository root. Do not assume the tool runner inherits an interactive shell's direnv environment. This applies to builds, tests, generators, formatters, package installation, and development servers.
- `.envrc` uses the flake dev shell in `flake.nix`, which supplies the project toolchain and Playwright's Chromium. Do not bypass it with host tools, global package installs, downloaded browsers, or ad hoc library-path fixes.
- Examples: `direnv exec . just generate-db`, `direnv exec . bun run --cwd ui typecheck`, and `direnv exec . scripts/test-postgres.sh go test -tags=integration ./internal/catalog`.
- For commands that require a subdirectory, load the root environment first: `direnv exec . bash -c 'cd ui && bunx playwright test e2e/core.spec.ts --grep storage'`.
- If direnv reports an unapproved `.envrc`, inspect `.envrc` and `flake.nix` and ask the user to approve it with `direnv allow`. Do not grant trust automatically or silently fall back to another environment.
- Keep secrets out of logs and commits. Do not print `.env`, dump the full environment, or expose credentials while diagnosing environment issues.

## Sources Of Truth

- Use `justfile` for supported workflows; it loads a root `.env` automatically.
- TypeSpec files in `typespec/` own the HTTP contract. Do not hand-edit `openapi/teldrive.openapi.yaml`, `internal/api/gen/`, or `ui/src/api/schema.ts`; run `just generate-api`, `just generate-ui`, or `just generate` as appropriate.
- SQL lives in `db/queries/` and migrations in `db/migrations/`. Do not edit `internal/db/sqlcgen/` manually; always run `just generate-db` after SQL changes. Bare `sqlc generate` omits the required `patchsqlc` step and breaks configurable PostgreSQL schema rewriting.
- Go config structs in `internal/config` own the NixOS/Home Manager option schema. Do not hand-edit `nix/modules/generated-options.nix` or `nix/modules/generated-leaf-maps.nix`; run `just nix-generate`.
- The same Go config structs own `docs/content/docs/configuration/reference.mdx`; do not hand-edit it. Run `just docs-generate`.
- Preserve `/* TEMPLATE: schema */` markers in SQL; the generated DB wrapper replaces them with the configured schema at runtime.

## Commands

All commands below require the `direnv exec .` prefix described above.

- Install pinned JS dependencies: `just install-tools` (uses Bun for `typespec/`, `ui/`, and `docs/`).
- Backend unit tests: `go test ./...`; focused package/test: `go test ./internal/transfer -run '^TestName$'`.
- Integration tests must use the PostgreSQL harness: `scripts/test-postgres.sh go test -tags=integration ./internal/uploads`; use `just test-integration` for all packages. The harness prefers Podman and falls back to Docker only when Podman is not installed.
- Race tests also require the PostgreSQL harness: `just test-race`.
- Full project validation: `just check`. This regenerates artifacts and runs lint, UI checks/build, docs build, unit tests, and PostgreSQL-harness-backed integration tests; it is intentionally expensive.
- UI checks: `just ui-check` (includes browser E2E with mocked API responses).
- Documentation: edit pages in `docs/content/docs/`; use `just docs-dev` to preview or `just docs-build` to generate references and build the site.
- Format only handwritten code with `just format`; generated Go directories are deliberately excluded.

## Change And Verification Rules

- Inspect `git status` before editing; preserve unrelated user changes. Keep edits scoped to the request and do not commit or push unless asked.
- Before non-trivial changes, identify the behavior being changed and the smallest tests that can prove it. Add regression coverage for bug fixes and new behavior, including relevant boundary cases.
- For UI changes, run typechecking, lint, and a production build. Run the relevant browser specs explicitly; the default UI test command runs a selected list, not every spec. For storage changes, include `e2e/core.spec.ts --grep storage` on desktop and mobile.
- Use the flake-provided `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH`. If the browser fails to launch, check that the command ran through direnv before changing dependencies or installing browsers.
- For SQL changes, regenerate with `just generate-db` and run the affected integration tests through `scripts/test-postgres.sh`. Cover date-window boundaries, zero-data cases, and user isolation when changing statistics queries.
- Reuse the existing UI components, theme, and localization system. New or changed visible labels must be updated in every supported locale; interactive charts must support keyboard and touch input as well as pointer input.
- Describe metrics according to their actual data source. Do not label active-file metadata as historical upload traffic or Telegram part counts as byte usage.
- Review the final diff, including generated artifacts. Report the checks actually run and their outcomes; distinguish environment blockers from code failures. Never claim unrun or blocked checks passed.

## Architecture

- `cmd/teldrive` is the CLI entrypoint; `internal/app/app.go` is the composition root and owns migrations, services, HTTP routing, workers, and shutdown order.
- `internal/api` adapts the ogen contract; domain behavior belongs in packages such as `catalog`, `uploads`, `transfer`, `fileops`, and `shares`, not generated handlers.
- `internal/telegramstore` is the external storage boundary. Production uses gotd; integration and UI tests can use the filesystem backend.
- `db/migrations` includes application schema; startup also runs River/RiverPro migrations before opening the long-lived pool.
- The UI is a Vite/React app in `ui/`; `ui/ui.go` embeds `ui/dist`, so `just build` builds the UI before compiling the server binary.

## Generated Changes

- API generation intentionally fails if ogen emits an unimplemented-handler fallback or operation counts diverge; implement every generated handler method explicitly.
- Review generated diffs after `just generate`; generation can touch Go API code, SQL query code, OpenAPI, and the UI schema together.
