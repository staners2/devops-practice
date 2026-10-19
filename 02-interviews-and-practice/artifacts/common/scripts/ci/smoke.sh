#!/usr/bin/env bash
set -euo pipefail

for binary in api worker migrate smoke; do
  if [[ ! -x "bin/$binary" ]]; then
    echo "bin/$binary is required" >&2
    exit 1
  fi
done

migrated=false
for _ in $(seq 1 60); do
  if ./bin/migrate; then
    migrated=true
    break
  fi
  sleep 2
done
if [[ "$migrated" != true ]]; then
  echo "migrations did not become ready" >&2
  exit 1
fi

METRICS_ADDR=127.0.0.1:0 ./bin/api >api.log 2>&1 &
api_pid=$!
METRICS_ADDR=127.0.0.1:0 ./bin/worker >worker.log 2>&1 &
worker_pid=$!

cleanup() {
  local status=$?
  kill -TERM "$api_pid" "$worker_pid" 2>/dev/null || true
  wait "$api_pid" || status=$?
  wait "$worker_pid" || status=$?
  return "$status"
}
trap cleanup EXIT

./bin/smoke
health=$(curl -fsS http://127.0.0.1:8080/api/health-check)
printf '%s\n' "$health" | grep -Fq '"postgres":"ok"'
printf '%s\n' "$health" | grep -Fq '"redis":"ok"'
printf '%s\n' "$health" | grep -Fq '"kafka":"ok"'
