#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${ROOT_DIR}/tests/docker-compose.test.yml"

export TEST_DATABASE_URL="${TEST_DATABASE_URL:-postgres://teldrive_test:teldrive_test@localhost:55432/teldrive_test?sslmode=disable}"

cleanup() {
  docker compose -f "${COMPOSE_FILE}" down -v --remove-orphans >/dev/null 2>&1 || true
}

trap cleanup EXIT

docker compose -f "${COMPOSE_FILE}" up -d
docker compose -f "${COMPOSE_FILE}" ps

echo "waiting for postgres health..."
for _ in $(seq 1 60); do
  status="$(docker inspect --format='{{if .State.Health}}{{.State.Health.Status}}{{else}}unknown{{end}}' teldrive-test-postgres 2>/dev/null || true)"
  if [[ "${status}" == "healthy" ]]; then
    break
  fi
  sleep 1
done

if [[ "$(docker inspect --format='{{if .State.Health}}{{.State.Health.Status}}{{else}}unknown{{end}}' teldrive-test-postgres 2>/dev/null || true)" != "healthy" ]]; then
  echo "postgres container is not healthy"
  exit 1
fi

go test -v ./tests/integration/... -count=1
