#!/bin/sh
set -eu

: "${DATABASE_URL:?DATABASE_URL must be set}"
: "${1:?usage: CONFIRM_RESTORE=YES ./scripts/restore-db.sh path/to/backup.dump}"

if [ "${CONFIRM_RESTORE:-}" != "YES" ]; then
  printf '%s\n' 'Refusing to restore without CONFIRM_RESTORE=YES.' >&2
  exit 1
fi

backup_file=$1
if [ ! -f "$backup_file" ]; then
  printf 'Backup file does not exist: %s\n' "$backup_file" >&2
  exit 1
fi

pg_restore --clean --if-exists --no-owner --dbname="$DATABASE_URL" "$backup_file"
printf 'Restored database backup: %s\n' "$backup_file"
