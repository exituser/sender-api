#!/bin/sh
set -eu

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

if [ -n "${DATABASE_URL:-}" ]; then
  pg_restore --clean --if-exists --no-owner --dbname="$DATABASE_URL" "$backup_file"
else
  compose_file=${COMPOSE_FILE:-docker-compose.yml}
  compose_service=${COMPOSE_DB_SERVICE:-db}
  docker compose -f "$compose_file" exec -T "$compose_service" \
    pg_restore --clean --if-exists --no-owner \
      -U "${POSTGRES_USER:-supabase_admin}" -d "${POSTGRES_DB:-sender_api}" \
    < "$backup_file"
fi
printf 'Restored database backup: %s\n' "$backup_file"
