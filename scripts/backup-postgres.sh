#!/bin/sh
set -eu

output_dir=${BACKUP_DIR:-backups}
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
output_file="$output_dir/sender-api-$timestamp.dump"

umask 077
mkdir -p "$output_dir"

if [ -n "${DATABASE_URL:-}" ]; then
  pg_dump --format=custom --no-owner --file="$output_file" "$DATABASE_URL"
  pg_restore --list "$output_file" >/dev/null
else
  compose_file=${COMPOSE_FILE:-docker-compose.yml}
  compose_service=${COMPOSE_DB_SERVICE:-db}
  docker compose -f "$compose_file" exec -T "$compose_service" \
    pg_dump --format=custom --no-owner \
      -U "${POSTGRES_USER:-supabase_admin}" -d "${POSTGRES_DB:-sender_api}" \
    > "$output_file"
  # Validate through the same Compose image, so the host needs only Docker.
  docker compose -f "$compose_file" exec -T "$compose_service" \
    pg_restore --list < "$output_file" >/dev/null
fi
printf 'Created database backup: %s\n' "$output_file"
