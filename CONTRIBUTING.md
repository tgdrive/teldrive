# Contributing to Teldrive

Bug reports, documentation improvements, and pull requests are welcome. For questions or ideas, join the [Discord community](https://discord.gg/8QAeCvTK7G).

## Report a bug or suggest a feature

Search [existing issues](https://github.com/tgdrive/teldrive/issues) before opening a new one. For bugs, include the Teldrive version, deployment method, steps to reproduce, expected behavior, and relevant logs. Remove tokens, passwords, session data, and other private information.

For larger features or architectural changes, open an issue to discuss the scope before implementing them.

## Set up development

Fork the repository and clone your fork, then choose one of the setups below. Run commands from the repository root unless noted otherwise.

### With Nix and direnv

Install Nix with flakes enabled and direnv. The repository's `.envrc` loads the toolchain defined in `flake.nix`, including Go, Bun, Just, SQLC, PostgreSQL, Podman, and Chromium for browser tests.

Inspect `.envrc` and `flake.nix` before approving the environment:

```bash
direnv allow
direnv exec . just install-tools
```

With this setup, run project commands through `direnv exec .`; do not assume a non-interactive shell has loaded the environment. The dev shell configures Playwright to use its Chromium, so no separate browser download is needed.

### Without Nix

Install these tools using their official installers or your platform's package manager:

- Go 1.26 or later, matching the minimum version in `go.mod`.
- Bun, Node.js, Git, Just, and SQLC. See `flake.nix` for the development toolchain used by this repository.
- PostgreSQL to run the application locally.
- Podman for integration tests, or Docker if Podman is not installed. The test harness prefers Podman whenever it is present; make sure your selected runtime is working.
- FFmpeg for media-related development and browser test fixtures.

Nix and direnv are not required for this manual setup. The checked-in `.envrc` is Nix-specific; do not approve it without Nix or change it just for this setup. Install the pinned JavaScript dependencies, then install Playwright's Chromium:

```bash
just install-tools
cd ui
bunx playwright install chromium
cd ..
```

On supported Linux distributions, use `bunx playwright install --with-deps chromium` from `ui/` to also install browser system dependencies; this may require administrator privileges. See [Playwright's browser setup](https://playwright.dev/docs/browsers).

Leave `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` unset to use Playwright's downloaded browser. If using an existing compatible Chromium instead, set it to that browser's absolute executable path.

The remaining examples use the Nix/direnv prefix. **For manual, non-Nix development, omit `direnv exec .`**: for example, run `just check` or `bash -c 'cd ui && bunx playwright test'` directly. Repository automation and coding agents should continue to follow the environment rules in [AGENTS.md](AGENTS.md).

### Run the application

To run the application locally, configure PostgreSQL and your Telegram application credentials using the [documentation](https://tgdrive.github.io/teldrive/). The `justfile` loads a root `.env` automatically. Never commit secrets or use production data for tests.

```bash
direnv exec . just dev
```

## Make and verify changes

Create a branch, keep the change focused, and add regression tests for new behavior or bug fixes. Reuse existing UI components and update every supported locale when changing visible text.

Useful checks:

```bash
# Format handwritten code.
direnv exec . just format

# Backend unit tests.
direnv exec . just test-unit

# PostgreSQL-backed integration tests.
direnv exec . just test-integration

# UI generation, build, lint, typechecking, and selected browser tests.
direnv exec . just ui-check

# Complete browser suite, including specs outside the default UI test command.
direnv exec . bash -c 'cd ui && bunx playwright test'

# Full project validation, including generated artifacts and documentation.
direnv exec . just check
```

Integration tests must use the PostgreSQL harness rather than an arbitrary database. For a focused run:

```bash
direnv exec . scripts/test-postgres.sh go test -tags=integration ./internal/catalog
```

## Generated files

Edit sources, not generated output, and include the regenerated artifacts in your pull request:

| Source changed | Generation command |
| --- | --- |
| HTTP contracts in `typespec/` | `direnv exec . just generate-api` and `direnv exec . just generate-ui` |
| SQL queries or migrations in `db/` | `direnv exec . just generate-db` |
| Configuration structs in `internal/config` | `direnv exec . just nix-generate` and `direnv exec . just docs-generate` |

Use `direnv exec . just generate` to run all generators. Do not run bare `sqlc generate`: the repository target also applies the required schema-rewriting patch. Preserve `/* TEMPLATE: schema */` markers in SQL and review generated diffs before submitting.

Documentation pages live in `docs/content/docs/`. Preview them with `direnv exec . just docs-dev` and validate with `direnv exec . just docs-build`.

## Submit a pull request

- Explain what changed and why; link related issues.
- List the checks you ran and disclose any failures or checks you could not run.
- Include screenshots for visual changes when useful.
- Exclude secrets, unrelated changes, and local build or test artifacts.
