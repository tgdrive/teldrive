#!/usr/bin/env bash
set -euo pipefail

minimum="${COVERAGE_MIN:-80}"
profile="${COVERAGE_PROFILE:-coverage.out}"

# The percentage gate measures deterministic business logic. The full test
# suite still runs, including generated-router, application-lifecycle, and
# boundary-adapter tests. Generated code, composition-only wiring, and the live
# gotd RPC adapter are not denominator padding for the core-logic metric.
core_patterns=(
  ./internal/authn
  ./internal/bots
  ./internal/catalog
  ./internal/channels
  ./internal/config
  ./internal/contentcrypto
  ./internal/database
  ./internal/dbtypes
  ./internal/fileops
  ./internal/health
  ./internal/jobs
  ./internal/principal
  ./internal/secureblob
  ./internal/shares
  ./internal/transfer
  ./internal/treehash
  ./internal/uploads
)
mapfile -t core_packages < <(go list "${core_patterns[@]}")
coverpkg="$(IFS=,; echo "${core_packages[*]}")"

./scripts/test-postgres.sh go test \
  -tags=integration \
  -covermode=atomic \
  -coverpkg="$coverpkg" \
  -coverprofile="$profile" \
  ./...

total="$(go tool cover -func="$profile" | awk '/^total:/ {gsub(/%/, "", $3); print $3}')"
printf 'core business coverage: %s%% (minimum %s%%)\n' "$total" "$minimum"

awk -v total="$total" -v minimum="$minimum" 'BEGIN {
  if ((total + 0) < (minimum + 0)) {
    printf "coverage %.1f%% is below required %.1f%%\n", total, minimum > "/dev/stderr"
    exit 1
  }
}'
