#!/bin/sh
set -eu
name=${0##*/}
printf '%s %s\n' "$name" "$*" >> "$TEST_CALLS"
case "$name" in
  pg_dump)
    [ "${1:-}" != --version ] || { printf 'pg_dump (PostgreSQL) 18.6\n'; exit 0; }
    [ "${TEST_DUMP_FAIL:-0}" = 0 ] || exit 8
    for arg do case "$arg" in --file=*) printf 'fixture dump\n' > "${arg#--file=}" ;; esac; done
    ;;
  psql)
    query=$(cat)
    printf '%s\n' "$query" >> "$TEST_SQL"
    case "$query" in
      *json_build_object*)
        for arg do case "$arg" in backup_id=*) id=${arg#backup_id=} ;; esac; done
        printf '{"backup_id" : "%s", "system_identifier" : "1111111111111111111"}\n' "$id"
        ;;
      *pg_control_system*) printf '%s\n' "${TEST_SYSTEM_MATCH:-t}" ;;
      *'SELECT count(*) FROM pg_database'*) printf '%s\n' "${TEST_EXISTS:-0}" ;;
      *) printf 'fixture query completed\n' ;;
    esac
    ;;
  pg_restore) [ "${TEST_RESTORE_FAIL:-0}" = 0 ] || exit 9 ;;
  rm)
    case "$*" in *database.dump*) [ "${TEST_RETENTION_FAIL:-0}" = 0 ] || exit 10 ;; esac
    /bin/rm "$@"
    ;;
  *) exit 2 ;;
esac
