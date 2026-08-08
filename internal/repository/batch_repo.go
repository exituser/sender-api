package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sender-api/sender-api/internal/domain"
)

type BatchRepo struct {
	db *pgxpool.Pool
}

func NewBatchRepo(db *pgxpool.Pool) *BatchRepo {
	return &BatchRepo{db: db}
}

func (r *BatchRepo) Ensure(ctx context.Context, teamID uuid.UUID, key, requestHash string) (bool, error) {
	var created bool
	err := r.db.QueryRow(ctx, `
		INSERT INTO email_batches (team_id, idempotency_key, request_hash)
		VALUES ($1, $2, $3)
		ON CONFLICT (team_id, idempotency_key) DO UPDATE
		SET request_hash = email_batches.request_hash
		WHERE email_batches.request_hash = EXCLUDED.request_hash
		RETURNING (xmax = 0)
	`, teamID, key, requestHash).Scan(&created)
	if err == nil {
		return created, nil
	}

	var existingHash string
	if lookupErr := r.db.QueryRow(ctx, `
		SELECT request_hash FROM email_batches WHERE team_id = $1 AND idempotency_key = $2
	`, teamID, key).Scan(&existingHash); lookupErr != nil {
		return false, lookupErr
	}
	if existingHash != requestHash {
		return false, domain.ErrBatchRequestConflict
	}
	return false, nil
}
