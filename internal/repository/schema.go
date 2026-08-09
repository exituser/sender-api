package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const ExpectedSchemaVersion int64 = 13

func CheckSchemaVersion(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return fmt.Errorf("database pool is unavailable")
	}
	var version int64
	var dirty bool
	if err := db.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0), COALESCE(BOOL_OR(dirty), false)
		FROM public.schema_migrations
	`).Scan(&version, &dirty); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if dirty || version != ExpectedSchemaVersion {
		return fmt.Errorf("schema version must be clean version %d (got version=%d dirty=%t)", ExpectedSchemaVersion, version, dirty)
	}
	return nil
}
