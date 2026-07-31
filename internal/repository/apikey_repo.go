package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sender-api/sender-api/internal/auth"
	"github.com/sender-api/sender-api/internal/domain"
)

type APIKeyRepo struct {
	db *pgxpool.Pool
}

func NewAPIKeyRepo(db *pgxpool.Pool) *APIKeyRepo {
	return &APIKeyRepo{db: db}
}

func (r *APIKeyRepo) Create(ctx context.Context, key *domain.APIKey) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO api_keys (id, team_id, name, key_hash, key_prefix, permissions)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, key.ID, key.TeamID, key.Name, key.KeyHash, key.KeyPrefix, key.Permissions)
	return err
}

func (r *APIKeyRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.APIKey, error) {
	var k domain.APIKey
	err := r.db.QueryRow(ctx, `
		SELECT id, team_id, name, key_hash, key_prefix, permissions, created_at, last_used_at
		FROM api_keys WHERE id = $1
	`, id).Scan(&k.ID, &k.TeamID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.Permissions, &k.CreatedAt, &k.LastUsedAt)
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func (r *APIKeyRepo) GetByHash(ctx context.Context, hash string) (*domain.APIKey, error) {
	var k domain.APIKey
	err := r.db.QueryRow(ctx, `
		SELECT id, team_id, name, key_hash, key_prefix, permissions, created_at, last_used_at
		FROM api_keys WHERE key_hash = $1
	`, hash).Scan(&k.ID, &k.TeamID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.Permissions, &k.CreatedAt, &k.LastUsedAt)
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func (r *APIKeyRepo) List(ctx context.Context, teamID uuid.UUID) ([]domain.APIKey, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, team_id, name, key_hash, key_prefix, permissions, created_at, last_used_at
		FROM api_keys WHERE team_id = $1
		ORDER BY created_at DESC
	`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []domain.APIKey
	for rows.Next() {
		var k domain.APIKey
		err := rows.Scan(&k.ID, &k.TeamID, &k.Name, &k.KeyHash, &k.KeyPrefix, &k.Permissions, &k.CreatedAt, &k.LastUsedAt)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}

func (r *APIKeyRepo) DeleteForTeam(ctx context.Context, teamID, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM api_keys WHERE id = $1 AND team_id = $2`, id, teamID)
	return err
}

func (r *APIKeyRepo) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE api_keys
		SET last_used_at = NOW()
		WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < NOW() - INTERVAL '1 minute')
	`, id)
	return err
}

func (r *APIKeyRepo) VerifyAPIKey(ctx context.Context, rawKey string) (*domain.APIKeyVerification, error) {
	hash := auth.HashAPIKey(rawKey)
	var verification domain.APIKeyVerification
	var teamID, apiKeyID uuid.UUID
	err := r.db.QueryRow(ctx, `
		SELECT k.team_id, k.id, k.permissions, t.plan
		FROM api_keys k
		JOIN teams t ON t.id = k.team_id
		WHERE k.key_hash = $1
	`, hash).Scan(
		&teamID,
		&apiKeyID,
		&verification.Permissions,
		&verification.Plan,
	)
	if err != nil {
		return nil, err
	}
	verification.TeamID = teamID.String()
	verification.APIKeyID = apiKeyID.String()
	return &verification, nil
}
