#!/bin/sh
set -eu
# shellcheck source=scripts/postgresql/common.sh
. "$(dirname "$0")/common.sh"
check_connection
: "${EXPECTED_PGHOST:?EXPECTED_PGHOST must be independently verified before restore}"
[ "$PGHOST" = "$EXPECTED_PGHOST" ] || fail 'restore host differs from the verified target'
check_directory
: "${BACKUP_ID:?BACKUP_ID is required}" "${RESTORE_DATABASE:?RESTORE_DATABASE is required}"
case "$RESTORE_DATABASE" in devops_practice_restore_?*) ;; *) fail 'restore requires a new devops_practice_restore_ database' ;; esac
case "$RESTORE_DATABASE" in *[!a-z0-9_]*) fail 'unsafe RESTORE_DATABASE' ;; esac
[ "${#RESTORE_DATABASE}" -le 63 ] && [ "$RESTORE_DATABASE" != "$PGDATABASE" ] || fail 'restore target must differ from the source database'
check_bundle
metadata=$(cat "$bundle/metadata.json")
same_cluster=$(PGDATABASE=postgres psql -X -v ON_ERROR_STOP=1 -At -v metadata="$metadata" <<'SQL'
SELECT system_identifier::text = (:'metadata'::json->>'system_identifier') FROM pg_control_system();
SQL
)
[ "$same_cluster" = t ] || fail 'restore server identity differs from the backup source; no changes made'
exists=$(PGDATABASE=postgres psql -X -v ON_ERROR_STOP=1 -At -v target="$RESTORE_DATABASE" <<'SQL'
SELECT count(*) FROM pg_database WHERE datname = :'target';
SQL
)
[ "$exists" = 0 ] || fail 'restore target already exists; no changes made'
PGDATABASE=postgres psql -X -v ON_ERROR_STOP=1 -v target="$RESTORE_DATABASE" <<'SQL'
CREATE DATABASE :"target" OWNER devops TEMPLATE template0;
SQL
PGDATABASE="$RESTORE_DATABASE" pg_restore --single-transaction --exit-on-error \
  --no-owner --no-acl --role=devops --dbname="$RESTORE_DATABASE" "$bundle/database.dump"
PGDATABASE="$RESTORE_DATABASE" psql -X -v ON_ERROR_STOP=1 -v target="$RESTORE_DATABASE" <<'SQL'
ANALYZE;
SELECT current_database(), version FROM schema_migrations ORDER BY version;
SELECT count(*) AS restored_requests FROM requests;
SQL
printf 'restore complete: %s; validate known rows before business smoke\n' "$RESTORE_DATABASE"
