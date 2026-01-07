# TelDrive Agent Guidelines

This file contains instructions for agentic coding agents working on the TelDrive codebase.

## 1. Build, Lint, and Test Commands

The project uses `task` (Taskfile.yml) for build automation and standard Go toolchain.

### Build
- **Build Server:** `task server` (builds binary to `bin/teldrive`)
- **Build UI:** `task ui` (downloads frontend assets)
- **Full Build:** `task` (runs `ui` then `server`)
- **Install Dependencies:** `task deps` (runs `go mod download && go mod tidy`)
- **Generate Code:** `task gen` (runs `go generate ./...`)

### Test
- **Run All Tests:** `go test ./...`
- **Run Specific Package:** `go test ./internal/config/...`
- **Run Specific Test:** `go test -v ./internal/config -run TestConfigLoader`
- **Run with Race Detector:** `go test -race ./...`

### Lint
- **Run Linter:** `task lint` (runs `golangci-lint run`)
- **Format Code:** `go fmt ./...`

## 2. Code Style & Guidelines

### Formatting & Style
- **Go Standard:** Strictly follow `gofmt` and standard Go idioms.
- **Imports:** Group imports into standard library, 3rd party, and local packages.
- **Line Length:** Aim for readable line lengths (soft limit ~120 chars), but prioritize readability.

### Project Structure
- **`cmd/`**: Entry points for the application commands (server, check, etc.).
- **`internal/`**: Private application code.
  - **`config/`**: Configuration loading (Koanf based).
  - **`database/`**: Database interactions (GORM).
  - **`tgc/`**: Telegram client wrappers.
- **`pkg/`**: Library code that might be imported by other projects.
  - **`services/`**: Core business logic and API handlers.
  - **`models/`**: GORM database models.

### Naming Conventions
- **Interfaces:** Named with `-er` suffix (e.g., `Reader`, `Writer`) where appropriate.
- **Structs:** PascalCase.
- **Variables:** camelCase.
- **Constants:** PascalCase or SCREAMING_SNAKE_CASE.
- **Package Names:** Short, lowercase, single word (e.g., `config`, `auth`).

### Error Handling
- **Wrapping:** Use `%w` to wrap errors to preserve context.
- **Check Errors:** Never swallow errors. Handle them or return them.
- **Panic:** Avoid panic except during initialization/startup where recovery is impossible.
- **Logging:** Use `zap` logger via `logging.FromContext(ctx)`.
  - **Levels:** Use `Debug` for trace info, `Info` for general ops, `Error` for failures.
  - **Fields:** Use structured logging (e.g., `zap.String("key", "value")`).

### Configuration
- **Library:** Uses `knadh/koanf/v2`.
- **Loading:** Loads from Defaults -> Config File -> Env Vars -> Flags.
- **Validation:** Uses `go-playground/validator`. Ensure struct tags `validate:"..."` are present.
- **Mapping:** Use explicit mapping in `populate` method in `internal/config/config.go` (avoid reflection where possible).

### Database
- **Library:** GORM with PostgreSQL.
- **Migrations:** Use SQL migrations in `internal/database/migrations` or GORM auto-migration if configured.
- **Context:** Always pass `ctx` to database calls.

### Telegram Client (TGC)
- **Library:** `gotd/td`.
- **Session:** Managed via `internal/tgc` and database.
- **Concurrency:** Be mindful of Telegram rate limits. Use `tgc.NewMiddleware` with rate limiters.

### Frontend
- **Location:** `ui/` directory.
- **Assets:** Embeds frontend in binary via `embed` package or serves from `ui/dist`.

## 3. Development Workflow
1.  **Dependencies:** Ensure `task` is installed.
2.  **Generate:** Run `task gen` if modifying API definitions or generated code.
3.  **Test:** Write unit tests for new logic, especially in `internal/` packages.
4.  **Lint:** Run linter before committing.
