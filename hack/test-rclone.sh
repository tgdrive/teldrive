#!/usr/bin/env bash
set -euo pipefail

image="${TELDRIVE_POSTGRES_IMAGE:-ghcr.io/tgdrive/postgres:18}"
name="teldrive-rclone-${USER:-user}-$$"
user="teldrive"
password="teldrive"
database="teldrive_rclone"
api_key="${TELDRIVE_RCLONE_API_KEY:-tdk_rclone_local_integration}"
rclone_repo="${RCLONE_REPO:-$(cd "$(dirname "$0")/../.." && pwd)/rclone}"
tmp_dir="$(mktemp -d)"
server_pid=""

cleanup() {
  if [[ -n "$server_pid" ]]; then
    kill "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" >/dev/null 2>&1 || true
  fi
  podman rm -f "$name" >/dev/null 2>&1 || true
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

command -v podman >/dev/null 2>&1 || {
  echo "podman is required" >&2
  exit 1
}
command -v curl >/dev/null 2>&1 || {
  echo "curl is required" >&2
  exit 1
}
[[ -f "$rclone_repo/go.mod" && -d "$rclone_repo/backend/teldrive" ]] || {
  echo "rclone repository not found at $rclone_repo" >&2
  exit 1
}

podman run -d --name "$name" \
  -e POSTGRES_USER="$user" \
  -e POSTGRES_PASSWORD="$password" \
  -e POSTGRES_DB="$database" \
  -p 127.0.0.1::5432 \
  "$image" >/dev/null

postgres_port="$(podman port "$name" 5432/tcp | awk -F: '{print $NF}')"
if [[ -z "$postgres_port" ]]; then
  echo "failed to determine PostgreSQL port" >&2
  podman logs "$name" >&2 || true
  exit 1
fi

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

database_url="postgres://${user}:${password}@127.0.0.1:${postgres_port}/${database}?sslmode=disable"
http_port="$(python3 - <<'PY'
import socket
s = socket.socket()
s.bind(('127.0.0.1', 0))
print(s.getsockname()[1])
s.close()
PY
)"
api_host="http://127.0.0.1:${http_port}"

mkdir -p "$tmp_dir/telegram"
go build -o "$tmp_dir/teldrive" ./cmd/teldrive

TELDRIVE_DATABASE_URL="$database_url" \
TELDRIVE_HTTP_ADDRESS="127.0.0.1:${http_port}" \
TELDRIVE_TELEGRAM_BACKEND="filesystem" \
TELDRIVE_TELEGRAM_LOCAL_ROOT="$tmp_dir/telegram" \
TELDRIVE_SECURITY_SIGNING_KEY="0123456789abcdef0123456789abcdef" \
TELDRIVE_SECURITY_DATA_KEY="AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" \
TELDRIVE_LOGGING_LOG_LEVEL="debug" \
TELDRIVE_LOGGING_LOG_FORMAT="text" \
"$tmp_dir/teldrive" run >"$tmp_dir/server.log" 2>&1 &
server_pid="$!"

ready=false
for _ in $(seq 1 200); do
  if curl --fail --silent --show-error "$api_host/health/ready" >/dev/null 2>&1; then
    ready=true
    break
  fi
  if ! kill -0 "$server_pid" >/dev/null 2>&1; then
    echo "TelDrive exited before becoming ready" >&2
    cat "$tmp_dir/server.log" >&2
    exit 1
  fi
  sleep 0.1
done
if [[ "$ready" != true ]]; then
  echo "TelDrive did not become ready" >&2
  cat "$tmp_dir/server.log" >&2
  exit 1
fi

api_key_hash="$(printf '%s' "$api_key" | sha256sum | awk '{print $1}')"
api_key_prefix="${api_key:0:16}"
podman exec -i -e PGPASSWORD="$password" "$name" \
  psql -v ON_ERROR_STOP=1 -h 127.0.0.1 -U "$user" -d "$database" >/dev/null <<SQL
INSERT INTO users (user_id, display_name, username)
VALUES (1001, 'Rclone Integration', 'rclone')
ON CONFLICT (user_id) DO NOTHING;

INSERT INTO api_keys (user_id, name, key_prefix, secret_hash)
VALUES (1001, 'rclone integration', '${api_key_prefix}', decode('${api_key_hash}', 'hex'))
ON CONFLICT (secret_hash) DO NOTHING;
SQL

matched_keys="$(podman exec -e PGPASSWORD="$password" "$name" psql -h 127.0.0.1 -U "$user" -d "$database" -Atqc "SELECT count(*) FROM api_keys WHERE secret_hash = decode('${api_key_hash}', 'hex') AND revoked_at IS NULL")"
if [[ "$matched_keys" != "1" ]]; then
  echo "seeded API key hash was not found in PostgreSQL (count=$matched_keys)" >&2
  exit 1
fi

if ! curl --fail --silent --show-error -H "X-API-Key: $api_key" "$api_host/api/v1/me" >"$tmp_dir/me.json"; then
  echo "seeded API key authentication failed" >&2
  cat "$tmp_dir/server.log" >&2
  exit 1
fi

if ! (
  cd "$rclone_repo"
  RCLONE_CONFIG_TESTTELDRIVE_TYPE="teldrive" \
  RCLONE_CONFIG_TESTTELDRIVE_API_HOST="$api_host" \
  RCLONE_CONFIG_TESTTELDRIVE_API_KEY="$api_key" \
  RCLONE_CONFIG_TESTTELDRIVE_CHUNK_SIZE="64M" \
  RCLONE_CONFIG_TESTTELDRIVE_UPLOAD_CONCURRENCY="4" \
  RCLONE_CONFIG_TESTTELDRIVE_HASH_ENABLED="true" \
  go test ./backend/teldrive -run "${RCLONE_TEST_RUN:-^Test(Integration|ResumeIntegration|CaseAndTrashIntegration)$}" -count=1 -v
); then
  echo "rclone integration failed; TelDrive log follows" >&2
  cat "$tmp_dir/server.log" >&2
  exit 1
fi
