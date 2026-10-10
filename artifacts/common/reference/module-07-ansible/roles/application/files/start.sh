#!/usr/bin/env bash
set -euo pipefail

[[ $# -eq 1 && "$1" =~ ^http://[0-9.]+:8080$ ]] || { printf 'Некорректный локальный URL приложения.\n' >&2; exit 2; }
api_url=$1
compose=(docker compose --project-name devops-practice-ansible-07 \
  --env-file .env --file compose.yaml --file compose.vm.yaml)

if ! timeout --signal=TERM --kill-after=5s 210s "${compose[@]}" up -d --wait --wait-timeout 180 postgres redis kafka >/dev/null 2>&1; then
  printf 'Не удалось подготовить зависимости приложения.\n' >&2
  exit 1
fi
if ! timeout --signal=TERM --kill-after=5s 90s "${compose[@]}" run --rm --no-deps kafka-init >/dev/null 2>&1; then
  printf 'Не удалось подготовить Kafka topic.\n' >&2
  exit 1
fi
if ! timeout --signal=TERM --kill-after=5s 90s "${compose[@]}" run --rm --no-deps migrate >/dev/null 2>&1; then
  printf 'Не удалось применить миграции.\n' >&2
  exit 1
fi
if ! timeout --signal=TERM --kill-after=5s 30s "${compose[@]}" up -d --no-deps api worker >/dev/null 2>&1; then
  printf 'Не удалось запустить API/worker.\n' >&2
  exit 1
fi
for ((attempt=0; attempt<20; attempt++)); do
  if curl --fail --silent --max-time 3 "$api_url/api/health-check" >/dev/null; then
    exit 0
  fi
  sleep 2
done
printf 'API не прошёл health-check.\n' >&2
exit 1
