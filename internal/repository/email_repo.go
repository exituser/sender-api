package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sender-api/sender-api/internal/domain"
)

type EmailRepo struct {
	db *pgxpool.Pool
}

func NewEmailRepo(db *pgxpool.Pool) *EmailRepo {
	return &EmailRepo{db: db}
}

func (r *EmailRepo) Create(ctx context.Context, email *domain.Email) error {
	toJSON, _ := json.Marshal(email.To)
	ccJSON, _ := json.Marshal(email.CC)
	bccJSON, _ := json.Marshal(email.BCC)
	replyToJSON, _ := json.Marshal(email.ReplyTo)
	attachmentsJSON, _ := json.Marshal(email.Attachments)
	tagsJSON, _ := json.Marshal(email.Tags)
	metadataJSON, _ := json.Marshal(email.Metadata)
	headersJSON, _ := json.Marshal(email.Headers)

	_, err := r.db.Exec(ctx, `
		INSERT INTO emails (id, team_id, api_key_id, from_addr, to_addr, cc, bcc, subject, html, text, reply_to, attachments, status, tags, metadata, headers, idempotency_key, idempotency_hash, scheduled_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
	`,
		email.ID, email.TeamID, email.APIKeyID, email.From,
		toJSON, ccJSON, bccJSON,
		email.Subject, email.HTML, email.Text, replyToJSON, attachmentsJSON, email.Status,
		tagsJSON, metadataJSON, headersJSON, email.IdempotencyKey, email.IdempotencyHash, email.ScheduledAt,
	)
	return err
}

func (r *EmailRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Email, error) {
	var email domain.Email
	var toJSON, ccJSON, bccJSON, replyToJSON, attachmentsJSON, tagsJSON, metadataJSON, headersJSON []byte

	err := r.db.QueryRow(ctx, `
		SELECT id, team_id, api_key_id, from_addr, to_addr, cc, bcc, subject, html, text, reply_to, attachments, status, tags, metadata, headers, idempotency_key, idempotency_hash, provider_message_id, sending_at, scheduled_at, sent_at, created_at
		FROM emails WHERE id = $1
	`, id).Scan(
		&email.ID, &email.TeamID, &email.APIKeyID, &email.From,
		&toJSON, &ccJSON, &bccJSON,
		&email.Subject, &email.HTML, &email.Text, &replyToJSON, &attachmentsJSON, &email.Status,
		&tagsJSON, &metadataJSON, &headersJSON, &email.IdempotencyKey, &email.IdempotencyHash, &email.ProviderMessageID, &email.SendingAt,
		&email.ScheduledAt, &email.SentAt, &email.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal(toJSON, &email.To)
	_ = json.Unmarshal(ccJSON, &email.CC)
	_ = json.Unmarshal(bccJSON, &email.BCC)
	_ = json.Unmarshal(replyToJSON, &email.ReplyTo)
	_ = json.Unmarshal(attachmentsJSON, &email.Attachments)
	_ = json.Unmarshal(tagsJSON, &email.Tags)
	_ = json.Unmarshal(metadataJSON, &email.Metadata)
	_ = json.Unmarshal(headersJSON, &email.Headers)

	return &email, nil
}

func (r *EmailRepo) GetByIDForTeam(ctx context.Context, teamID, id uuid.UUID) (*domain.Email, error) {
	email, err := r.GetByID(ctx, id)
	if err != nil || email.TeamID != teamID {
		return nil, fmt.Errorf("email not found")
	}
	return email, nil
}

func (r *EmailRepo) GetByIdempotencyKey(ctx context.Context, teamID uuid.UUID, key string) (*domain.Email, error) {
	var id uuid.UUID
	if err := r.db.QueryRow(ctx, `
		SELECT id FROM emails WHERE team_id = $1 AND idempotency_key = $2
	`, teamID, key).Scan(&id); err != nil {
		return nil, err
	}
	return r.GetByIDForTeam(ctx, teamID, id)
}

func (r *EmailRepo) GetByProviderMessageID(ctx context.Context, messageID string) (*domain.Email, error) {
	var id uuid.UUID
	if err := r.db.QueryRow(ctx, `
		SELECT id FROM emails WHERE provider_message_id = $1
	`, messageID).Scan(&id); err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

func (r *EmailRepo) SetProviderMessageID(ctx context.Context, id uuid.UUID, messageID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE emails SET provider_message_id = $1 WHERE id = $2
	`, messageID, id)
	return err
}

func (r *EmailRepo) ClaimForSending(ctx context.Context, id uuid.UUID) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE emails SET status = 'sending', sending_at = NOW()
		WHERE id = $1 AND status = 'queued'
	`, id)
	return tag.RowsAffected() == 1, err
}

func (r *EmailRepo) CancelQueued(ctx context.Context, teamID, id uuid.UUID) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE emails SET status = 'cancelled'
		WHERE id = $1 AND team_id = $2 AND status = 'queued'
	`, id, teamID)
	return tag.RowsAffected() == 1, err
}

func (r *EmailRepo) List(ctx context.Context, teamID uuid.UUID, limit, offset int) (*domain.EmailListResponse, error) {
	var total int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM emails WHERE team_id = $1`, teamID).Scan(&total)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT id, team_id, api_key_id, from_addr, to_addr, subject, status, created_at
		FROM emails WHERE team_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, teamID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var emails []domain.Email
	for rows.Next() {
		var email domain.Email
		var toJSON []byte
		err := rows.Scan(&email.ID, &email.TeamID, &email.APIKeyID, &email.From, &toJSON, &email.Subject, &email.Status, &email.CreatedAt)
		if err != nil {
			return nil, err
		}
		_ = json.Unmarshal(toJSON, &email.To)
		emails = append(emails, email)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &domain.EmailListResponse{
		Data:   emails,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func (r *EmailRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.EmailStatus) error {
	query := `UPDATE emails SET status = $1`
	if status == domain.EmailStatusSending {
		query += `, sending_at = NOW()`
	} else {
		query += `, sending_at = NULL`
	}
	if status == domain.EmailStatusSent {
		query += `, sent_at = NOW()`
	}
	query += ` WHERE id = $2`

	_, err := r.db.Exec(ctx, query, status, id)
	return err
}

func (r *EmailRepo) ResetSendingToQueued(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `
		UPDATE emails SET status = 'queued', sending_at = NULL
		WHERE status = 'sending'
		  AND (sending_at IS NULL OR sending_at < NOW() - INTERVAL '10 minutes')
	`)
	return err
}

func (r *EmailRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM emails WHERE id = $1`, id)
	return err
}

func (r *EmailRepo) DeleteForTeam(ctx context.Context, teamID, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM emails WHERE id = $1 AND team_id = $2`, id, teamID)
	return err
}

func (r *EmailRepo) AddEvent(ctx context.Context, event *domain.EmailEvent) error {
	dataJSON, _ := json.Marshal(event.Data)
	_, err := r.db.Exec(ctx, `
		INSERT INTO email_events (id, email_id, event, data, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, event.ID, event.EmailID, event.Event, dataJSON, event.IPAddress, event.UserAgent)
	return err
}

func (r *EmailRepo) GetEvents(ctx context.Context, emailID uuid.UUID) ([]domain.EmailEvent, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, email_id, event, timestamp, data, ip_address, user_agent
		FROM email_events WHERE email_id = $1
		ORDER BY timestamp DESC
	`, emailID)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []domain.EmailEvent
	for rows.Next() {
		var event domain.EmailEvent
		var dataJSON []byte
		err := rows.Scan(&event.ID, &event.EmailID, &event.Event, &event.Timestamp, &dataJSON, &event.IPAddress, &event.UserAgent)
		if err != nil {
			return nil, err
		}
		_ = json.Unmarshal(dataJSON, &event.Data)
		events = append(events, event)
	}
	return events, rows.Err()
}

func (r *EmailRepo) GetEventsForTeam(ctx context.Context, teamID, emailID uuid.UUID) ([]domain.EmailEvent, error) {
	rows, err := r.db.Query(ctx, `
		SELECT ev.id, ev.email_id, ev.event, ev.timestamp, ev.data, ev.ip_address, ev.user_agent
		FROM email_events ev
		JOIN emails e ON e.id = ev.email_id
		WHERE ev.email_id = $1 AND e.team_id = $2
		ORDER BY ev.timestamp DESC
	`, emailID, teamID)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []domain.EmailEvent
	for rows.Next() {
		var event domain.EmailEvent
		var dataJSON []byte
		if err := rows.Scan(&event.ID, &event.EmailID, &event.Event, &event.Timestamp, &dataJSON, &event.IPAddress, &event.UserAgent); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(dataJSON, &event.Data)
		events = append(events, event)
	}
	return events, rows.Err()
}
