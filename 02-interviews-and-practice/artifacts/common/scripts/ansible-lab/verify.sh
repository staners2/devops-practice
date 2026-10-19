#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
  printf 'Использование: bash verify.sh DEPLOY_DIR http://PROXY_IP [REQUEST_UUID]\n' >&2
  exit 2
fi
deploy_dir=$1
proxy_url=$2
request_id=${3:-}
[[ "$deploy_dir" == /opt/devops-practice-ansible-07 ]] || { printf 'Ожидается каталог стенда модуля 07.\n' >&2; exit 2; }
[[ "$proxy_url" =~ ^http://[a-zA-Z0-9.-]+(:[0-9]+)?$ ]] || { printf 'Некорректный HTTP-адрес proxy.\n' >&2; exit 2; }
if [[ -n "$request_id" && ! "$request_id" =~ ^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$ ]]; then
  printf 'Некорректный UUID заявки.\n' >&2
  exit 2
fi
request_id=${request_id,,}
compose=(docker compose --project-name devops-practice-ansible-07 \
  --env-file "$deploy_dir/.env" --file "$deploy_dir/compose.yaml" --file "$deploy_dir/compose.vm.yaml")

verify_deadline=$(( $(date +%s) + 130 ))
bounded() {
  local remaining=$(( verify_deadline - $(date +%s) ))
  if (( remaining <= 0 )); then
    printf 'Истёк общий таймаут проверки стенда.\n' >&2
    return 124
  fi
  timeout --signal=TERM --kill-after=5s "${remaining}s" "$@"
}

bounded timeout --signal=TERM --kill-after=5s 120s "${compose[@]}" run --rm --no-deps \
  --env "API_BASE_URL=$proxy_url" smoke

if [[ -n "$request_id" ]]; then
  # Переменные PostgreSQL раскрываются внутри контейнера, не на control node.
  # shellcheck disable=SC2016
  persisted=$(bounded "${compose[@]}" exec -T postgres sh -ec \
    'exec psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -v request_id="$1"' sh "$request_id" <<'SQL'
SELECT id::text || ' ' || status FROM requests WHERE id = :'request_id'::uuid;
SQL
  )
  [[ "$persisted" == "$request_id processed" ]] || { printf 'Заявка не найдена в PostgreSQL в статусе processed.\n' >&2; exit 1; }
  printf 'PostgreSQL: %s\n' "$persisted"
fi

containers=$(bounded "${compose[@]}" ps --quiet api worker postgres kafka)
[[ -n "$containers" ]] || { printf 'Контейнеры стенда отсутствуют.\n' >&2; exit 1; }
while IFS= read -r container; do
  bounded docker inspect --format '{{.Name}} image={{.Image}} state={{.State.Status}} {{range .Mounts}}volume={{.Name}} target={{.Destination}} {{end}}' "$container"
done <<< "$containers"
