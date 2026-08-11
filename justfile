set dotenv-load := true

openapi_spec := "openapi/teldrive.openapi.yaml"
ui_dir := "ui"
ogen_version := "v1.22.0"
binary := "bin/teldrive"
version := env_var_or_default("VERSION", "dev")
default_commit := `git rev-parse --short HEAD 2>/dev/null || echo unknown`
default_build_date := `date -u +%Y-%m-%dT%H:%M:%SZ`
commit := env_var_or_default("COMMIT", default_commit)
build_date := env_var_or_default("BUILD_DATE", default_build_date)
ldflags := "-s -w -X main.version=" + version + " -X main.commit=" + commit + " -X main.date=" + build_date

_default:
    @just --list

install-tools:
    bun ci --cwd typespec
    bun ci --cwd {{ui_dir}}

format:
    bun run --cwd typespec format
    bun run --cwd {{ui_dir}} format
    gofmt -w $(find . -name '*.go' -not -path './internal/api/gen/*' -not -path './internal/db/sqlcgen/*')

lint:
    bun run --cwd typespec lint
    bun run --cwd {{ui_dir}} lint
    go vet ./...

# TypeSpec is the source of truth for the HTTP contract.
generate-openapi:
    bun run --cwd typespec generate:openapi

# Generate the complete Go server/client contract.
generate-api: generate-openapi
    go run github.com/ogen-go/ogen/cmd/ogen@{{ogen_version}} \
        --config ogen.yml \
        --target internal/api/gen \
        --package gen \
        --clean \
        {{openapi_spec}}
    test ! -e internal/api/gen/oas_unimplemented_gen.go
    test "$(grep -c '^\s*[A-Z][A-Za-z0-9]*(ctx context.Context' internal/api/gen/oas_server_gen.go)" -eq "$(grep -c 'operationId:' {{openapi_spec}})"

# Generate the typed PostgreSQL query layer.
generate-db:
    sqlc generate
    python3 hack/patch-sqlc-schema.py

generate-ui: generate-openapi
    bun run --cwd {{ui_dir}} generate:api

generate: generate-api generate-db generate-ui
    go mod tidy

ui-install:
    bun ci --cwd {{ui_dir}}

ui-format:
    bun run --cwd {{ui_dir}} format:check

ui-lint:
    bun run --cwd {{ui_dir}} lint

ui-typecheck: generate-ui
    bun run --cwd {{ui_dir}} typecheck

ui-test:
    bun run --cwd {{ui_dir}} test

ui-build: generate-ui
    bun run --cwd {{ui_dir}} build

ui-dev:
    bun run --cwd {{ui_dir}} dev

ui-e2e:
    ./hack/test-ui.sh

ui-check: ui-format ui-lint ui-typecheck ui-test ui-build

build: ui-build
    mkdir -p bin
    CGO_ENABLED=0 go build -trimpath -ldflags '{{ldflags}}' -o {{binary}} ./cmd/teldrive

run:
    go run ./cmd/teldrive serve

image:
    podman build --build-arg VERSION={{version}} --build-arg COMMIT={{commit}} --build-arg BUILD_DATE={{build_date}} -t teldrive-backend:{{version}} .

test-unit:
    go test ./...

test-integration:
    ./hack/test-postgres.sh go test -tags=integration ./...

test-race:
    ./hack/test-postgres.sh go test -race -tags=integration ./...

coverage:
    ./hack/coverage.sh

check: generate lint ui-typecheck ui-test ui-build test-unit coverage

ci: check

clean-generated:
    rm -rf openapi internal/api/gen internal/db/sqlcgen ui/src/api/schema.ts ui/dist coverage.out
