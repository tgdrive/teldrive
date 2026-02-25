#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${ROOT_DIR}/tests/docker-compose.test.yml"
DB_USER="${JET_DB_USER:-teldrive_test}"
DB_PASS="${JET_DB_PASSWORD:-teldrive_test}"
DB_HOST="${JET_DB_HOST:-localhost}"
DB_PORT="${JET_DB_PORT:-55432}"
DB_NAME="${JET_DB_NAME:-teldrive_jet}"
DB_URL="postgres://${DB_USER}:${DB_PASS}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"

cleanup() {
  docker compose -f "${COMPOSE_FILE}" down -v --remove-orphans >/dev/null 2>&1 || true
}

trap cleanup EXIT

docker compose -f "${COMPOSE_FILE}" up -d

for _ in $(seq 1 60); do
  health="$(docker inspect --format='{{if .State.Health}}{{.State.Health.Status}}{{else}}unknown{{end}}' teldrive-test-postgres 2>/dev/null || true)"
  if [[ "${health}" == "healthy" ]]; then
    break
  fi
  sleep 1
done

if [[ "$(docker inspect --format='{{if .State.Health}}{{.State.Health.Status}}{{else}}unknown{{end}}' teldrive-test-postgres 2>/dev/null || true)" != "healthy" ]]; then
  echo "postgres container is not healthy"
  exit 1
fi

docker exec teldrive-test-postgres psql -U "${DB_USER}" -d postgres -tc "SELECT 1 FROM pg_database WHERE datname = '${DB_NAME}'" | grep -q 1 || \
  docker exec teldrive-test-postgres psql -U "${DB_USER}" -d postgres -c "CREATE DATABASE \"${DB_NAME}\""

go run github.com/pressly/goose/v3/cmd/goose@latest -dir "${ROOT_DIR}/internal/database/migrations" postgres "${DB_URL}" up

go run github.com/go-jet/jet/v2/cmd/jet@latest \
  -dsn="${DB_URL}" \
  -schema=teldrive \
  -path="${ROOT_DIR}/internal/database/jetgen"
