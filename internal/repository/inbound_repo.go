package repository

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sender-api/sender-api/internal/domain"
)

type InboundEmailRepo struct {
	db *pgxpool.Pool
}

func NewInboundEmailRepo(db *pgxpool.Pool) *InboundEmailRepo {
	return &InboundEmailRepo{db: db}
}

func (r *InboundEmailRepo) Create(ctx context.Context, email *domain.InboundEmail) error {
	toJSON, _ := json.Marshal(email.To)
	attJSON, _ := json.Marshal(email.Attachments)
	headersJSON, _ := json.Marshal(email.Headers)

	_, err := r.db.Exec(ctx, `
		INSERT INTO inbound_emails (id, team_id, message_id, from_addr, to_addr, subject, text, html, attachments, raw_s3_key, headers)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, email.ID, email.TeamID, email.MessageID, email.From, toJSON, email.Subject,
		email.Text, email.HTML, attJSON, email.RawS3Key, headersJSON)
	return err
}

func (r *InboundEmailRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.InboundEmail, error) {
	var email domain.InboundEmail
	var toJSON, attJSON, headersJSON []byte

	err := r.db.QueryRow(ctx, `
		SELECT id, team_id, message_id, from_addr, to_addr, subject, text, html, attachments, raw_s3_key, headers, created_at
		FROM inbound_emails WHERE id = $1
	`, id).Scan(
		&email.ID, &email.TeamID, &email.MessageID, &email.From,
		&toJSON, &email.Subject, &email.Text, &email.HTML,
		&attJSON, &email.RawS3Key, &headersJSON, &email.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	json.Unmarshal(toJSON, &email.To)
	json.Unmarshal(attJSON, &email.Attachments)
	json.Unmarshal(headersJSON, &email.Headers)

	return &email, nil
}

func (r *InboundEmailRepo) GetByMessageID(ctx context.Context, teamID uuid.UUID, messageID string) (*domain.InboundEmail, error) {
	var email domain.InboundEmail
	var toJSON, attJSON, headersJSON []byte

	err := r.db.QueryRow(ctx, `
		SELECT id, team_id, message_id, from_addr, to_addr, subject, text, html, attachments, raw_s3_key, headers, created_at
		FROM inbound_emails WHERE team_id = $1 AND message_id = $2
	`, teamID, messageID).Scan(
		&email.ID, &email.TeamID, &email.MessageID, &email.From,
		&toJSON, &email.Subject, &email.Text, &email.HTML,
		&attJSON, &email.RawS3Key, &headersJSON, &email.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal(toJSON, &email.To)
	_ = json.Unmarshal(attJSON, &email.Attachments)
	_ = json.Unmarshal(headersJSON, &email.Headers)
	return &email, nil
}

func (r *InboundEmailRepo) List(ctx context.Context, teamID uuid.UUID, limit, offset int) (*domain.InboundEmailListResponse, error) {
	var total int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM inbound_emails WHERE team_id = $1`, teamID).Scan(&total)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT id, team_id, message_id, from_addr, to_addr, subject, created_at
		FROM inbound_emails WHERE team_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, teamID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []domain.InboundEmail
	for rows.Next() {
		var email domain.InboundEmail
		var toJSON []byte
		err := rows.Scan(&email.ID, &email.TeamID, &email.MessageID, &email.From, &toJSON, &email.Subject, &email.CreatedAt)
		if err != nil {
			return nil, err
		}
		json.Unmarshal(toJSON, &email.To)
		emails = append(emails, email)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &domain.InboundEmailListResponse{
		Data:   emails,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}
