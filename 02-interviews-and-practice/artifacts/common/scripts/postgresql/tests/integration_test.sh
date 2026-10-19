#!/bin/sh
set -eu
repo_root=$(CDPATH='' cd -- "$(dirname "$0")/../../.." && pwd)
cd "$repo_root"
case "${PGDATABASE:-}" in *_test) ;; *) echo 'integration requires a dedicated *_test database' >&2; exit 1 ;; esac
: "${PGHOST:?}" "${PGUSER:?}" "${PGPASSWORD:?}"
temporary=$(mktemp -d)
temporary=$(CDPATH='' cd -- "$temporary" && pwd -P)
restore_database="devops_practice_restore_test_$(date -u +%Y%m%d%H%M%S)_$$"
restore_created=0
trap 'if [ "$restore_created" = 1 ]; then PGDATABASE=postgres psql -X -v ON_ERROR_STOP=1 -v target="$restore_database" -q <<EOF
DROP DATABASE IF EXISTS :"target";
EOF
fi
rm -rf "$temporary"' EXIT
existing=$(psql -X -At -v ON_ERROR_STOP=1 -c "SELECT to_regclass('public.requests') IS NOT NULL")
[ "$existing" = f ] || { echo 'test requests already exists; refusing to overwrite' >&2; exit 1; }
psql -X -v ON_ERROR_STOP=1 -q -f migrations/001_initial.sql
psql -X -v ON_ERROR_STOP=1 -q -f migrations/002_requests_status_created_at.sql
psql -X -v ON_ERROR_STOP=1 -q <<'SQL'
CREATE TABLE schema_migrations (version bigint PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());
INSERT INTO schema_migrations(version) VALUES (1), (2);
INSERT INTO requests VALUES ('11111111-1111-4111-8111-111111111111', 'restore integration', 'processed', now(), now());
SQL
mkdir "$temporary/backups"
export BACKUP_DIR="$temporary/backups" BACKUP_KEEP=3
sh scripts/postgresql/backup.sh
bundle=$(find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d)
export BACKUP_ID="${bundle##*/}" RESTORE_DATABASE="$restore_database"
export EXPECTED_PGHOST="$PGHOST"
sh scripts/postgresql/restore.sh
restore_created=1
PGDATABASE="$restore_database" psql -X -v ON_ERROR_STOP=1 \
  -v expected_database="$restore_database" -v known_id=11111111-1111-4111-8111-111111111111 \
  -f scripts/postgresql/verify-restore.sql
if sh scripts/postgresql/restore.sh > "$temporary/repeat.log" 2>&1; then
  echo 'restore modified existing target' >&2; exit 1
fi
test "$(psql -X -At -c 'SELECT count(*) FROM requests')" = 1
test "$(PGDATABASE="$restore_database" psql -X -At -c 'SELECT count(*) FROM schema_migrations')" = 2
for diagnostic in activity locks replication; do
  psql -X -v ON_ERROR_STOP=1 -f "scripts/postgresql/$diagnostic.sql" > "$temporary/$diagnostic.log"
done
printf 'Real PostgreSQL dump/restore, known row, source preservation and diagnostics passed\n'
