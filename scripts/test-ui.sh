#!/usr/bin/env bash
set -euo pipefail

mode="oneshot"
case "${1:-}" in
  start|test|stop|status)
    mode="$1"
    shift
    ;;
esac

image="${TELDRIVE_POSTGRES_IMAGE:-ghcr.io/tgdrive/postgres:18}"
state_dir="${TELDRIVE_UI_TEST_STATE_DIR:-${XDG_RUNTIME_DIR:-/tmp}/teldrive-ui-${USER:-user}}"
name="teldrive-ui-${USER:-user}-$$"
user="teldrive"
password="teldrive"
database="teldrive_ui"
api_key="${TELDRIVE_UI_TEST_API_KEY:-tdk_ui_local_integration}"
signing_key="0123456789abcdef0123456789abcdef"
session_id="aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
logout_session_id="bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
refresh_session_id="cccccccc-cccc-4ccc-8ccc-cccccccccccc"
error_session_id="dddddddd-dddd-4ddd-8ddd-dddddddddddd"
admin_session_id="eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
alice_session_id="11111111-aaaa-4111-8111-111111111111"
bob_session_id="22222222-bbbb-4222-8222-222222222222"
charlie_session_id="33333333-cccc-4333-8333-333333333333"
disabled_session_id="44444444-dddd-4444-8444-444444444444"
refresh_token="tdr_ui_default_session"
logout_refresh_token="tdr_ui_logout_session"
refresh_only_token="tdr_ui_refresh_session"
error_refresh_token="tdr_ui_error_session"
server_pid=""
preserve_environment=false

if [[ "$mode" == "start" ]]; then
  name="teldrive-ui-${USER:-user}-persistent"
  tmp_dir="$state_dir/runtime"
elif [[ "$mode" == "oneshot" ]]; then
  tmp_dir="$(mktemp -d)"
else
  tmp_dir=""
fi

cleanup() {
  if [[ "$preserve_environment" == true ]]; then
    return
  fi
  if [[ -n "$server_pid" ]]; then
    kill "$server_pid" > /dev/null 2>&1 || true
    wait "$server_pid" > /dev/null 2>&1 || true
  fi
  podman rm -f "$name" > /dev/null 2>&1 || true
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

for command in podman curl node bun python3; do
  command -v "$command" > /dev/null 2>&1 || { echo "$command is required" >&2; exit 1; }
done

run_playwright() {
  local playwright_version playwright_image
  playwright_version="$(node -p "require('./ui/node_modules/@playwright/test/package.json').version")"
  playwright_image="${TELDRIVE_PLAYWRIGHT_IMAGE:-mcr.microsoft.com/playwright:v${playwright_version}-noble}"

  if ! podman run --rm --network host --userns=keep-id \
    -v "$PWD:/work:Z" \
    -w /work/ui \
    -e TELDRIVE_UI_EXTERNAL_SERVER=1 \
    -e TELDRIVE_UI_BASE_URL="$base_url" \
    -e TELDRIVE_UI_TEST_API_KEY="$api_key" \
    -e TELDRIVE_UI_TEST_ACCESS_TOKEN="$access_token" \
    -e TELDRIVE_UI_TEST_REFRESH_TOKEN="$refresh_token" \
    -e TELDRIVE_UI_TEST_LOGOUT_ACCESS_TOKEN="$logout_access_token" \
    -e TELDRIVE_UI_TEST_LOGOUT_REFRESH_TOKEN="$logout_refresh_token" \
    -e TELDRIVE_UI_TEST_EXPIRED_ACCESS_TOKEN="$expired_access_token" \
    -e TELDRIVE_UI_TEST_REFRESH_ONLY_TOKEN="$refresh_only_token" \
    -e TELDRIVE_UI_TEST_ERROR_ACCESS_TOKEN="$error_access_token" \
    -e TELDRIVE_UI_TEST_ERROR_REFRESH_TOKEN="$error_refresh_token" \
    -e TELDRIVE_UI_TEST_ADMIN_ACCESS_TOKEN="$admin_access_token" \
    -e TELDRIVE_UI_TEST_ALICE_ACCESS_TOKEN="$alice_access_token" \
    -e TELDRIVE_UI_TEST_BOB_ACCESS_TOKEN="$bob_access_token" \
    -e TELDRIVE_UI_TEST_CHARLIE_ACCESS_TOKEN="$charlie_access_token" \
    -e TELDRIVE_UI_TEST_DISABLED_ACCESS_TOKEN="$disabled_access_token" \
    "$playwright_image" \
    npx playwright test "$@"; then
    echo "Playwright failed; Teldrive log follows" >&2
    cat "$tmp_dir/server.log" >&2
    return 1
  fi
}

if [[ "$mode" == "stop" ]]; then
  if [[ ! -f "$state_dir/environment" ]]; then
    echo "No persistent UI test environment is running."
    exit 0
  fi
  source "$state_dir/environment"
  preserve_environment=true
  kill "$server_pid" > /dev/null 2>&1 || true
  podman rm -f "$name" > /dev/null 2>&1 || true
  rm -rf "$state_dir"
  echo "Stopped the persistent UI test environment."
  exit 0
fi

if [[ "$mode" == "status" || "$mode" == "test" ]]; then
  if [[ ! -f "$state_dir/environment" ]]; then
    echo "No persistent UI test environment found. Run '$0 start' first." >&2
    exit 1
  fi
  source "$state_dir/environment"
  preserve_environment=true
  if ! kill -0 "$server_pid" > /dev/null 2>&1 || ! curl --fail --silent "$base_url/health/ready" > /dev/null; then
    echo "The persistent UI test environment is stale. Run '$0 stop' and '$0 start'." >&2
    exit 1
  fi
  if [[ "$mode" == "status" ]]; then
    echo "UI test environment is running at $base_url (PID $server_pid)."
    exit 0
  fi
  run_playwright "$@"
  exit $?
fi

if [[ "$mode" == "start" ]]; then
  if [[ -f "$state_dir/environment" ]]; then
    echo "A persistent UI test environment already exists. Run '$0 status' or '$0 stop'." >&2
    exit 1
  fi
  mkdir -p "$tmp_dir"
fi

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
    psql -h 127.0.0.1 -U "$user" -d "$database" -Atqc 'SELECT 1' 2>/dev/null | grep -qx '1'; then
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
base_url="http://127.0.0.1:${http_port}"

bun ci --cwd ui --ignore-scripts
VITE_TELDRIVE_REQUEST_TIMEOUT_MS="${TELDRIVE_UI_REQUEST_TIMEOUT_MS:-1500}" bun run --cwd ui build
go build -o "$tmp_dir/teldrive" ./cmd/teldrive
mkdir -p "$tmp_dir/telegram"

TELDRIVE_DATABASE_URL="$database_url" \
TELDRIVE_HTTP_ADDRESS="127.0.0.1:${http_port}" \
TELDRIVE_TELEGRAM_BACKEND="filesystem" \
TELDRIVE_TELEGRAM_LOCAL_ROOT="$tmp_dir/telegram" \
TELDRIVE_SECURITY_SIGNING_KEY="$signing_key" \
TELDRIVE_SECURITY_DATA_KEY="AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" \
TELDRIVE_LOGGING_LOG_LEVEL="debug" \
TELDRIVE_LOGGING_LOG_FORMAT="text" \
"$tmp_dir/teldrive" run >"$tmp_dir/server.log" 2>&1 &
server_pid="$!"

ready=false
for _ in $(seq 1 240); do
  if curl --fail --silent --show-error "$base_url/health/ready" >/dev/null 2>&1; then
    ready=true
    break
  fi
  if ! kill -0 "$server_pid" >/dev/null 2>&1; then
    echo "Teldrive exited before becoming ready" >&2
    cat "$tmp_dir/server.log" >&2
    exit 1
  fi
  sleep 0.1
done
if [[ "$ready" != true ]]; then
  echo "Teldrive did not become ready" >&2
  cat "$tmp_dir/server.log" >&2
  exit 1
fi

api_key_hash="$(printf '%s' "$api_key" | sha256sum | awk '{print $1}')"
api_key_prefix="${api_key:0:16}"
refresh_hash="$(printf '%s' "$refresh_token" | sha256sum | awk '{print $1}')"
logout_refresh_hash="$(printf '%s' "$logout_refresh_token" | sha256sum | awk '{print $1}')"
refresh_only_hash="$(printf '%s' "$refresh_only_token" | sha256sum | awk '{print $1}')"
error_refresh_hash="$(printf '%s' "$error_refresh_token" | sha256sum | awk '{print $1}')"
mapfile -t browser_tokens < <(SIGNING_KEY="$signing_key" SESSION_ID="$session_id" LOGOUT_SESSION_ID="$logout_session_id" REFRESH_SESSION_ID="$refresh_session_id" ERROR_SESSION_ID="$error_session_id" ADMIN_SESSION_ID="$admin_session_id" ALICE_SESSION_ID="$alice_session_id" BOB_SESSION_ID="$bob_session_id" CHARLIE_SESSION_ID="$charlie_session_id" DISABLED_SESSION_ID="$disabled_session_id" python3 - <<'PYJWT'
import base64
import hashlib
import hmac
import json
import os
import time

key = os.environ["SIGNING_KEY"].encode()
now = int(time.time())

def encoded(value):
    raw = json.dumps(value, separators=(",", ":"), sort_keys=True).encode()
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()

def token(user_id, session_id, expires_at):
    header = encoded({"alg": "HS256", "typ": "JWT"})
    payload = encoded({
        "iss": "teldrive-v2",
        "sub": str(user_id),
        "iat": now,
        "exp": expires_at,
        "sid": session_id,
        "roles": ["user"],
    })
    signing_input = f"{header}.{payload}"
    signature = base64.urlsafe_b64encode(
        hmac.new(key, signing_input.encode(), hashlib.sha256).digest()
    ).rstrip(b"=").decode()
    return f"{signing_input}.{signature}"

print(token(1001, os.environ["SESSION_ID"], now + 3600))
print(token(1001, os.environ["LOGOUT_SESSION_ID"], now + 3600))
print(token(1001, os.environ["REFRESH_SESSION_ID"], now - 60))
print(token(1001, os.environ["ERROR_SESSION_ID"], now + 3600))
print(token(1002, os.environ["ADMIN_SESSION_ID"], now + 3600))
print(token(1003, os.environ["ALICE_SESSION_ID"], now + 3600))
print(token(1004, os.environ["BOB_SESSION_ID"], now + 3600))
print(token(1005, os.environ["CHARLIE_SESSION_ID"], now + 3600))
print(token(1006, os.environ["DISABLED_SESSION_ID"], now + 3600))
PYJWT
)
access_token="${browser_tokens[0]}"
logout_access_token="${browser_tokens[1]}"
expired_access_token="${browser_tokens[2]}"
error_access_token="${browser_tokens[3]}"
admin_access_token="${browser_tokens[4]}"
alice_access_token="${browser_tokens[5]}"
bob_access_token="${browser_tokens[6]}"
charlie_access_token="${browser_tokens[7]}"
disabled_access_token="${browser_tokens[8]}"

podman cp ui/e2e/fixtures/access-control-seed.sql "$name:/tmp/access-control-seed.sql" >/dev/null
podman exec -e PGPASSWORD="$password" "$name" \
  psql -v ON_ERROR_STOP=1 -h 127.0.0.1 -U "$user" -d "$database" -f /tmp/access-control-seed.sql >/dev/null
podman exec -i -e PGPASSWORD="$password" "$name" \
  psql -v ON_ERROR_STOP=1 -h 127.0.0.1 -U "$user" -d "$database" >/dev/null <<SQL

INSERT INTO api_keys (user_id, name, key_prefix, secret_hash)
VALUES (1001, 'playwright integration', '${api_key_prefix}', decode('${api_key_hash}', 'hex'))
ON CONFLICT (secret_hash) DO NOTHING;

INSERT INTO sessions (id, user_id, telegram_session, refresh_token_hash, expires_at)
VALUES
  ('${session_id}', 1001, decode('00', 'hex'), decode('${refresh_hash}', 'hex'), now() + interval '1 day'),
  ('${logout_session_id}', 1001, decode('00', 'hex'), decode('${logout_refresh_hash}', 'hex'), now() + interval '1 day'),
  ('${refresh_session_id}', 1001, decode('00', 'hex'), decode('${refresh_only_hash}', 'hex'), now() + interval '1 day'),
  ('${error_session_id}', 1001, decode('00', 'hex'), decode('${error_refresh_hash}', 'hex'), now() + interval '1 day'),
  ('${admin_session_id}', 1002, decode('00', 'hex'), decode('11', 'hex'), now() + interval '1 day'),
  ('${alice_session_id}', 1003, decode('00', 'hex'), decode('12', 'hex'), now() + interval '1 day'),
  ('${bob_session_id}', 1004, decode('00', 'hex'), decode('13', 'hex'), now() + interval '1 day'),
  ('${charlie_session_id}', 1005, decode('00', 'hex'), decode('14', 'hex'), now() + interval '1 day'),
  ('${disabled_session_id}', 1006, decode('00', 'hex'), decode('15', 'hex'), now() + interval '1 day')
ON CONFLICT (id) DO NOTHING;

INSERT INTO files (id, user_id, parent_id, name, normalized_name, kind, status, mod_time, created_at, updated_at, deleted_at)
VALUES
  ('11111111-1111-4111-8111-111111111111', 1001, NULL, 'Documents', 'Documents', 'folder', 'active', '2026-01-15T12:00:00Z', '2026-01-15T12:00:00Z', '2026-01-15T12:00:00Z', NULL),
  ('22222222-2222-4222-8222-222222222222', 1001, NULL, 'Empty', 'Empty', 'folder', 'active', '2026-01-15T12:00:00Z', '2026-01-15T12:00:00Z', '2026-01-15T12:00:00Z', NULL),
  ('33333333-3333-4333-8333-333333333333', 1001, NULL, 'Large folder', 'Large folder', 'folder', 'active', '2026-01-15T12:00:00Z', '2026-01-15T12:00:00Z', '2026-01-15T12:00:00Z', NULL),
  ('44444444-4444-4444-8444-444444444444', 1001, '11111111-1111-4111-8111-111111111111', 'Reports', 'Reports', 'folder', 'active', '2026-01-15T12:00:00Z', '2026-01-15T12:00:00Z', '2026-01-15T12:00:00Z', NULL),
  ('55555555-5555-4555-8555-555555555555', 1001, NULL, 'Trash sample', 'Trash sample', 'folder', 'trashed', '2026-01-15T12:00:00Z', '2026-01-15T12:00:00Z', '2026-01-15T12:00:00Z', '2026-01-15T12:00:00Z')
ON CONFLICT (id) DO NOTHING;

INSERT INTO files (id, user_id, parent_id, name, normalized_name, kind, status, mod_time, created_at, updated_at)
SELECT gen_random_uuid(), 1001, '33333333-3333-4333-8333-333333333333',
       'Page item ' || lpad(value::text, 3, '0'),
       'Page item ' || lpad(value::text, 3, '0'),
       'folder', 'active', '2026-01-15T12:00:00Z', '2026-01-15T12:00:00Z', '2026-01-15T12:00:00Z'
FROM generate_series(1, 215) AS value;
SQL

if ! curl --fail --silent --show-error -H "X-API-Key: $api_key" "$base_url/api/v1/me" >/dev/null; then
  echo "seeded API key authentication failed" >&2
  cat "$tmp_dir/server.log" >&2
  exit 1
fi
if ! curl --fail --silent --show-error "$base_url/" | grep -q 'Teldrive'; then
  echo "built UI was not served from the Go application" >&2
  cat "$tmp_dir/server.log" >&2
  exit 1
fi

if [[ "$mode" == "start" ]]; then
  {
    printf 'name=%q\n' "$name"
    printf 'tmp_dir=%q\n' "$tmp_dir"
    printf 'server_pid=%q\n' "$server_pid"
    printf 'base_url=%q\n' "$base_url"
    printf 'api_key=%q\n' "$api_key"
    printf 'access_token=%q\n' "$access_token"
    printf 'refresh_token=%q\n' "$refresh_token"
    printf 'logout_access_token=%q\n' "$logout_access_token"
    printf 'logout_refresh_token=%q\n' "$logout_refresh_token"
    printf 'expired_access_token=%q\n' "$expired_access_token"
    printf 'refresh_only_token=%q\n' "$refresh_only_token"
    printf 'error_access_token=%q\n' "$error_access_token"
    printf 'error_refresh_token=%q\n' "$error_refresh_token"
    printf 'admin_access_token=%q\n' "$admin_access_token"
    printf 'alice_access_token=%q\n' "$alice_access_token"
    printf 'bob_access_token=%q\n' "$bob_access_token"
    printf 'charlie_access_token=%q\n' "$charlie_access_token"
    printf 'disabled_access_token=%q\n' "$disabled_access_token"
  } > "$state_dir/environment"
  chmod 600 "$state_dir/environment"
  preserve_environment=true
  echo "UI test environment is running at $base_url."
  echo "Run '$0 test [playwright arguments]' to reuse it and '$0 stop' when finished."
  exit 0
fi

run_playwright "$@"
