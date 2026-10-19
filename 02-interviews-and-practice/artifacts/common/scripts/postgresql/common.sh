#!/bin/sh

fail() { printf '%s\n' "$1" >&2; exit 1; }

check_connection() {
  : "${PGHOST:?PGHOST is required}" "${PGUSER:?PGUSER is required}" "${PGDATABASE:?PGDATABASE is required}"
  [ -n "${PGPASSWORD:-}${PGPASSFILE:-}" ] || fail 'PGPASSWORD or PGPASSFILE is required'
  PGPORT=${PGPORT:-5432}
  PGCONNECT_TIMEOUT=${PGCONNECT_TIMEOUT:-10}
  export PGPORT PGCONNECT_TIMEOUT
}

check_directory() {
  BACKUP_DIR=${BACKUP_DIR:?BACKUP_DIR is required}
  case "$BACKUP_DIR" in
    /|/tmp|/var|/var/lib|/var/lib/postgresql|/Users|/home|*'/../'*|*'/./'*|*/..|*/.|*/|*[!a-zA-Z0-9_./-]*) fail 'unsafe BACKUP_DIR' ;;
    /*) ;;
    *) fail 'BACKUP_DIR must be absolute' ;;
  esac
  [ -d "$BACKUP_DIR" ] && [ ! -L "$BACKUP_DIR" ] || fail 'BACKUP_DIR must exist and not be a symlink'
  [ "$(cd "$BACKUP_DIR" && pwd -P)" = "$BACKUP_DIR" ] || fail 'BACKUP_DIR must have no symlink or traversal components'
}

check_id() {
  printf '%s\n' "$1" | LC_ALL=C grep -Eq '^[0-9]{8}T[0-9]{6}Z-[0-9a-f]{16}$' || fail 'invalid BACKUP_ID'
}

check_bundle() {
  BACKUP_ID=${BACKUP_ID:?BACKUP_ID is required}
  check_id "$BACKUP_ID"
  bundle="$BACKUP_DIR/$BACKUP_ID"
  [ -d "$bundle" ] && [ ! -L "$bundle" ] || fail 'backup bundle not found or is a symlink'
  for bundle_file in database.dump SHA256SUMS metadata.json; do
    [ -f "$bundle/$bundle_file" ] && [ ! -L "$bundle/$bundle_file" ] && [ -s "$bundle/$bundle_file" ] || fail 'incomplete backup bundle'
  done
  [ "$(wc -l < "$bundle/SHA256SUMS" | tr -d ' ')" = 1 ] || fail 'invalid checksum manifest'
  LC_ALL=C grep -Eq '^[0-9a-f]{64}  database\.dump$' "$bundle/SHA256SUMS" || fail 'invalid checksum manifest'
  (cd "$bundle" && sha256sum --check --status SHA256SUMS) || fail 'backup checksum mismatch'
}
