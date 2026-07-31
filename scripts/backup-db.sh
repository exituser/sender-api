#!/bin/sh
set -eu

: "${DATABASE_URL:?DATABASE_URL must be set}"

output_dir=${BACKUP_DIR:-backups}
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
output_file="$output_dir/sender-api-$timestamp.dump"

umask 077
mkdir -p "$output_dir"
pg_dump --format=custom --no-owner --file="$output_file" "$DATABASE_URL"

printf 'Created database backup: %s\n' "$output_file"
