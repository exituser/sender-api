package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sender-api/sender-api/internal/domain"
)

type SuppressionRepo struct {
	db *pgxpool.Pool
}

func NewSuppressionRepo(db *pgxpool.Pool) *SuppressionRepo {
	return &SuppressionRepo{db: db}
}

func (r *SuppressionRepo) Upsert(ctx context.Context, suppression *domain.Suppression) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO suppressions (id, team_id, email, reason)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (team_id, email) DO UPDATE
		SET reason = CASE
			WHEN suppressions.reason = 'complaint' OR EXCLUDED.reason = 'complaint' THEN 'complaint'
			WHEN suppressions.reason = 'bounce' OR EXCLUDED.reason = 'bounce' THEN 'bounce'
			ELSE 'unsubscribe'
		END,
		updated_at = NOW()
	`, suppression.ID, suppression.TeamID, domain.NormalizeEmail(suppression.Email), suppression.Reason)
	return err
}

func (r *SuppressionRepo) GetByEmails(ctx context.Context, teamID uuid.UUID, emails []string) ([]domain.Suppression, error) {
	if len(emails) == 0 {
		return nil, nil
	}
	canonical := make([]string, 0, len(emails))
	seen := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		normalized := domain.NormalizeEmail(email)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		canonical = append(canonical, normalized)
	}
	if len(canonical) == 0 {
		return nil, nil
	}

	rows, err := r.db.Query(ctx, `
		SELECT id, team_id, email, reason, created_at, updated_at
		FROM suppressions
		WHERE team_id = $1 AND email = ANY($2)
	`, teamID, canonical)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	suppressions := make([]domain.Suppression, 0)
	for rows.Next() {
		var suppression domain.Suppression
		if err := rows.Scan(
			&suppression.ID,
			&suppression.TeamID,
			&suppression.Email,
			&suppression.Reason,
			&suppression.CreatedAt,
			&suppression.UpdatedAt,
		); err != nil {
			return nil, err
		}
		suppressions = append(suppressions, suppression)
	}
	return suppressions, rows.Err()
}
