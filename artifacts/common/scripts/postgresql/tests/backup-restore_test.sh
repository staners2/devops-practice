#!/bin/sh
set -eu
root=$(CDPATH='' cd -- "$(dirname "$0")/../../.." && pwd)
temporary=$(mktemp -d)
temporary=$(cd "$temporary" && pwd -P)
trap '/bin/rm -rf "$temporary"' EXIT
mkdir "$temporary/bin" "$temporary/backups"
cp "$root/scripts/postgresql/tests/client-stub.sh" "$temporary/bin/client"
chmod +x "$temporary/bin/client"
for client in pg_dump pg_restore psql rm; do ln -s client "$temporary/bin/$client"; done
PATH="$temporary/bin:$PATH"
export PATH
export PGHOST=fixture PGPORT=5432 PGUSER=postgres PGDATABASE=devops_practice PGPASSWORD=must_not_be_logged
export EXPECTED_PGHOST=fixture
export BACKUP_DIR="$temporary/backups" TEST_CALLS="$temporary/calls" TEST_SQL="$temporary/sql"
touch "$TEST_CALLS" "$TEST_SQL"
backup="$root/scripts/postgresql/backup.sh"
restore="$root/scripts/postgresql/restore.sh"
expect_failure() {
  if "$@" > "$temporary/output" 2>&1; then printf 'expected failure: %s\n' "$*" >&2; exit 1; fi
}
sh "$backup" > "$temporary/output"
BACKUP_ID=$(basename "$(find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d | head -n 1)")
export BACKUP_ID RESTORE_DATABASE=devops_practice_restore_fixture
sh "$restore" >> "$temporary/output"
grep -q -- '--single-transaction --exit-on-error --no-owner --no-acl --role=devops' "$TEST_CALLS"
test "$(grep -c 'CREATE DATABASE' "$TEST_SQL")" -eq 1
create_count=$(grep -c 'CREATE DATABASE' "$TEST_SQL")
expect_failure env TEST_SYSTEM_MATCH=f sh "$restore"
expect_failure env PGHOST=another-server sh "$restore"
test "$(grep -c 'CREATE DATABASE' "$TEST_SQL")" -eq "$create_count"
expect_failure env TEST_DUMP_FAIL=1 sh "$backup"
test -s "$BACKUP_DIR/$BACKUP_ID/database.dump"
expect_failure env TEST_RESTORE_FAIL=1 sh "$restore"
expect_failure env TEST_EXISTS=1 sh "$restore"
expect_failure env RESTORE_DATABASE=devops_practice sh "$restore"
expect_failure env RESTORE_DATABASE=postgres sh "$restore"
expect_failure env BACKUP_ID=../outside sh "$restore"
cp "$BACKUP_DIR/$BACKUP_ID/database.dump" "$temporary/original"
printf 'corrupted\n' >> "$BACKUP_DIR/$BACKUP_ID/database.dump"
expect_failure sh "$restore"
cp "$temporary/original" "$BACKUP_DIR/$BACKUP_ID/database.dump"
ln -s "$BACKUP_DIR" "$temporary/link"
expect_failure env BACKUP_DIR="$temporary/link" sh "$restore"
expect_failure env TEST_RETENTION_FAIL=1 BACKUP_KEEP=1 sh "$backup"
test -s "$BACKUP_DIR/$BACKUP_ID/database.dump"
if grep -q 'must_not_be_logged' "$TEST_CALLS" "$TEST_SQL" "$temporary/output"; then
  printf 'credentials leaked\n' >&2; exit 1
fi
printf 'backup/restore safety checks passed (mock clients, no real PostgreSQL)\n'
