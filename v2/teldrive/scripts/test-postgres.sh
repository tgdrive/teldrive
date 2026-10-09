#!/usr/bin/env bash
set -euo pipefail

image="${TELDRIVE_POSTGRES_IMAGE:-ghcr.io/tgdrive/postgres:18}"
name="teldrive-test-${USER:-user}-$$"
user="teldrive"
password="teldrive"
database="teldrive_test"

cleanup() {
  podman rm -f "$name" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

command -v podman >/dev/null 2>&1 || {
  echo "podman is required" >&2
  exit 1
}

podman run -d --name "$name" \
  -e POSTGRES_USER="$user" \
  -e POSTGRES_PASSWORD="$password" \
  -e POSTGRES_DB="$database" \
  -p 127.0.0.1::5432 \
  "$image" >/dev/null

port="$(podman port "$name" 5432/tcp | awk -F: '{print $NF}')"
if [[ -z "$port" ]]; then
  echo "failed to determine PostgreSQL port" >&2
  podman logs "$name" >&2 || true
  exit 1
fi

# The official PostgreSQL entrypoint starts a temporary Unix-socket-only server
# during initialization and then restarts PostgreSQL normally. Probing the local
# socket can therefore report ready too early. Requiring a TCP SQL query avoids
# that restart race and verifies the configured user and database as well.
ready=false
for _ in $(seq 1 120); do
  if podman exec -e PGPASSWORD="$password" "$name" \
    psql -h 127.0.0.1 -U "$user" -d "$database" -Atqc 'SELECT 1' \
    2>/dev/null | grep -qx '1'; then
    ready=true
    break
  fi
  sleep 0.25
done

if [[ "$ready" != true ]]; then
  echo "PostgreSQL did not become ready" >&2
  podman logs "$name" >&2 || true
  exit 1
fi

export TEST_DATABASE_URL="postgres://${user}:${password}@127.0.0.1:${port}/${database}?sslmode=disable"

if [[ "$#" -eq 0 ]]; then
  set -- go test -tags=integration ./...
fi

"$@"
