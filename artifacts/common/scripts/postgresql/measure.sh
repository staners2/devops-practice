#!/bin/sh
set -eu
# shellcheck source=scripts/postgresql/common.sh
. "$(dirname "$0")/common.sh"
check_connection
case "$PGDATABASE" in devops_practice_benchmark_?*) ;; *) fail 'measure requires a separate benchmark database' ;; esac
case "$PGDATABASE" in *[!a-z0-9_]*) fail 'unsafe benchmark database' ;; esac
command -v jq >/dev/null || fail 'jq is required'
root=$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)
OUTPUT_DIR=${OUTPUT_DIR:-"$root/.cache/postgresql-benchmark"}
mkdir -p "$OUTPUT_DIR"
exists=$(psql -X -v ON_ERROR_STOP=1 -At -c "SELECT to_regclass('public.requests') IS NOT NULL")
[ "$exists" = f ] || fail 'benchmark database must have no requests table; choose a new database'
psql -X -v ON_ERROR_STOP=1 -f "$root/migrations/001_initial.sql"
psql -X -v ON_ERROR_STOP=1 -f "$root/scripts/postgresql/seed.sql"
for phase in baseline optimized; do
  if [ "$phase" = optimized ]; then
    psql -X -v ON_ERROR_STOP=1 -f "$root/migrations/002_requests_status_created_at.sql"
    psql -X -v ON_ERROR_STOP=1 -c 'ANALYZE requests'
  fi
  for iteration in 1 2 3 4 5; do
    psql -X -v ON_ERROR_STOP=1 -At -f "$root/scripts/postgresql/explain.sql" > "$OUTPUT_DIR/$phase-$iteration.json"
    psql -X -v ON_ERROR_STOP=1 -qAt > "$OUTPUT_DIR/$phase-write-$iteration.json" <<'SQL'
BEGIN;
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)
UPDATE requests SET status = 'created'
WHERE id IN (SELECT id FROM requests WHERE title LIKE 'devops-practice-benchmark-05:%' AND status = 'processed' LIMIT 1000);
ROLLBACK;
SQL
  done
  jq -s '[.[][0]["Execution Time"]] | sort | .[2]' "$OUTPUT_DIR/$phase-"[1-5].json > "$OUTPUT_DIR/$phase-median-ms.json"
  jq -s '[.[][0]["Execution Time"]] | sort | .[2]' "$OUTPUT_DIR/$phase-write-"[1-5].json > "$OUTPUT_DIR/$phase-write-median-ms.json"
done
psql -X -v ON_ERROR_STOP=1 -c "SELECT indexrelname, pg_size_pretty(pg_relation_size(indexrelid)) FROM pg_stat_user_indexes WHERE relname = 'requests'"
printf 'Actual read/write plans and median milliseconds: %s; compare buffers and index size\n' "$OUTPUT_DIR"
