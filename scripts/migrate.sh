#!/usr/bin/env bash
set -euo pipefail

: "${DATABASE_URL:?DATABASE_URL is required}"

script_dir="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"
repo_root="$(CDPATH='' cd -- "$script_dir/.." && pwd)"

psql_cmd=(psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -X)

"${psql_cmd[@]}" <<'SQL'
CREATE TABLE IF NOT EXISTS public.schema_migrations (
    version BIGINT NOT NULL PRIMARY KEY,
    dirty BOOLEAN NOT NULL
);
SQL

migrations=()
while IFS= read -r migration; do
	migrations+=("$migration")
done < <(
	find "$repo_root/migrations" -maxdepth 1 -type f -name '*_*.up.sql' -exec basename {} \; |
		sort -t_ -k1,1n
)
for migration in "${migrations[@]}"; do
    version="${migration%%_*}"
    if [[ ! "$version" =~ ^[0-9]+$ ]]; then
        continue
    fi

	state="$("${psql_cmd[@]}" -Atc "SELECT COALESCE(MAX(version), 0) || '|' || COALESCE(BOOL_OR(dirty), false) FROM public.schema_migrations;")"
	current="${state%%|*}"
	dirty="${state##*|}"
	if [[ "$dirty" == "t" ]]; then
		echo "schema_migrations is dirty; repair the database before continuing" >&2
		exit 1
	fi
    if (( 10#$version <= current )); then
        continue
    fi

    echo "applying migration $migration"
    "${psql_cmd[@]}" <<SQL
BEGIN;
INSERT INTO public.schema_migrations (version, dirty) VALUES ($((10#$version)), true)
ON CONFLICT (version) DO UPDATE SET dirty = true;
\i '$repo_root/migrations/$migration'
UPDATE public.schema_migrations SET dirty = false WHERE version = $((10#$version));
COMMIT;
SQL
done

"${psql_cmd[@]}" -Atc "SELECT version || '|' || dirty FROM public.schema_migrations ORDER BY version DESC LIMIT 1;"
