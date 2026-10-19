#!/bin/sh
set -eu
umask 077
# shellcheck source=scripts/postgresql/common.sh
. "$(dirname "$0")/common.sh"
check_connection
check_directory
BACKUP_KEEP=${BACKUP_KEEP:-3}
case "$BACKUP_KEEP" in ''|*[!0-9]*) fail 'BACKUP_KEEP must be a positive integer' ;; esac
[ "$BACKUP_KEEP" -ge 1 ] && [ "$BACKUP_KEEP" -le 100 ] || fail 'BACKUP_KEEP must be between 1 and 100'

started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
backup_id="$(date -u +%Y%m%dT%H%M%SZ)-$(od -An -N8 -tx1 /dev/urandom | tr -d ' \n')"
check_id "$backup_id"
temporary=$(mktemp -d "$BACKUP_DIR/.partial-XXXXXXXX")
trap 'rm -f "$temporary/database.dump" "$temporary/SHA256SUMS" "$temporary/metadata.json" "$temporary/retention.list"; rmdir "$temporary"' EXIT
trap 'exit 1' HUP INT TERM

pg_dump --format=custom --file="$temporary/database.dump"
[ -s "$temporary/database.dump" ] || fail 'empty dump'
(cd "$temporary" && sha256sum database.dump > SHA256SUMS)
completed_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
size=$(wc -c < "$temporary/database.dump" | tr -d ' ')
client_version=$(pg_dump --version)
psql -X -v ON_ERROR_STOP=1 -At \
  -v backup_id="$backup_id" -v started_at="$started_at" -v completed_at="$completed_at" \
  -v size="$size" -v client_version="$client_version" > "$temporary/metadata.json" <<'SQL'
SELECT json_build_object('backup_id', :'backup_id', 'database', current_database(),
  'system_identifier', (SELECT system_identifier::text FROM pg_control_system()),
  'server_version', current_setting('server_version'), 'client_version', :'client_version',
  'started_at', :'started_at', 'completed_at', :'completed_at', 'size_bytes', :'size'::bigint);
SQL
[ -s "$temporary/metadata.json" ] || fail 'empty backup metadata'
[ ! -e "$BACKUP_DIR/$backup_id" ] || fail 'backup ID collision'
mv "$temporary" "$BACKUP_DIR/$backup_id"
trap - EXIT

# Only strict, completed bundles produced by this script enter retention.
retention_list=$(mktemp "$BACKUP_DIR/.retention-XXXXXXXX")
trap 'rm -f "$retention_list"' EXIT
for candidate in "$BACKUP_DIR"/*; do
  [ -d "$candidate" ] && [ ! -L "$candidate" ] || continue
  candidate_id=${candidate##*/}
  printf '%s\n' "$candidate_id" | LC_ALL=C grep -Eq '^[0-9]{8}T[0-9]{6}Z-[0-9a-f]{16}$' || continue
  [ -f "$candidate/metadata.json" ] && [ -f "$candidate/SHA256SUMS" ] && [ -f "$candidate/database.dump" ] || continue
  [ ! -L "$candidate/metadata.json" ] && [ ! -L "$candidate/SHA256SUMS" ] && [ ! -L "$candidate/database.dump" ] || continue
  [ "$candidate_id" != "$backup_id" ] || continue
  grep -Fq "\"backup_id\" : \"$candidate_id\"" "$candidate/metadata.json" || continue
  (BACKUP_ID="$candidate_id"; check_bundle) || fail 'retention found a corrupt completed backup'
  printf '%s\n' "$candidate_id"
done > "$retention_list"
LC_ALL=C sort -r "$retention_list" -o "$retention_list"
kept=1
while IFS= read -r candidate_id; do
  kept=$((kept + 1))
  [ "$kept" -le "$BACKUP_KEEP" ] && continue
  candidate="$BACKUP_DIR/$candidate_id"
  rm "$candidate/database.dump" "$candidate/SHA256SUMS" "$candidate/metadata.json"
  rmdir "$candidate"
done < "$retention_list"
printf 'backup complete: %s (%s bytes)\n' "$backup_id" "$size"
