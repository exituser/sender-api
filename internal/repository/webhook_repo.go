package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sender-api/sender-api/internal/domain"
)

type WebhookRepo struct {
	db *pgxpool.Pool
}

func NewWebhookRepo(db *pgxpool.Pool) *WebhookRepo {
	return &WebhookRepo{db: db}
}

func (r *WebhookRepo) Create(ctx context.Context, w *domain.Webhook) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO webhooks (id, team_id, url, events, secret, active)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, w.ID, w.TeamID, w.URL, w.Events, w.Secret, w.Active)
	return err
}

func (r *WebhookRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Webhook, error) {
	var w domain.Webhook
	err := r.db.QueryRow(ctx, `
		SELECT id, team_id, url, events, secret, active, created_at
		FROM webhooks WHERE id = $1
	`, id).Scan(&w.ID, &w.TeamID, &w.URL, &w.Events, &w.Secret, &w.Active, &w.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *WebhookRepo) GetByIDForTeam(ctx context.Context, teamID, id uuid.UUID) (*domain.Webhook, error) {
	w, err := r.GetByID(ctx, id)
	if err != nil || w.TeamID != teamID {
		return nil, fmt.Errorf("webhook not found")
	}
	return w, nil
}

func (r *WebhookRepo) List(ctx context.Context, teamID uuid.UUID) (*domain.WebhookListResponse, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, team_id, url, events, secret, active, created_at
		FROM webhooks WHERE team_id = $1
		ORDER BY created_at DESC
	`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var webhooks []domain.Webhook
	for rows.Next() {
		var w domain.Webhook
		err := rows.Scan(&w.ID, &w.TeamID, &w.URL, &w.Events, &w.Secret, &w.Active, &w.CreatedAt)
		if err != nil {
			return nil, err
		}
		webhooks = append(webhooks, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &domain.WebhookListResponse{Data: webhooks}, nil
}

func (r *WebhookRepo) Update(ctx context.Context, w *domain.Webhook) error {
	_, err := r.db.Exec(ctx, `
		UPDATE webhooks SET url = $1, events = $2, active = $3 WHERE id = $4
	`, w.URL, w.Events, w.Active, w.ID)
	return err
}

func (r *WebhookRepo) UpdateForTeam(ctx context.Context, w *domain.Webhook) error {
	_, err := r.db.Exec(ctx, `
		UPDATE webhooks SET url = $1, events = $2, active = $3 WHERE id = $4 AND team_id = $5
	`, w.URL, w.Events, w.Active, w.ID, w.TeamID)
	return err
}

func (r *WebhookRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM webhooks WHERE id = $1`, id)
	return err
}

func (r *WebhookRepo) DeleteForTeam(ctx context.Context, teamID, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM webhooks WHERE id = $1 AND team_id = $2`, id, teamID)
	return err
}

func (r *WebhookRepo) GetByEvent(ctx context.Context, teamID uuid.UUID, event string) ([]domain.Webhook, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, team_id, url, events, secret, active, created_at
		FROM webhooks WHERE team_id = $1 AND active = true AND $2 = ANY(events)
	`, teamID, event)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var webhooks []domain.Webhook
	for rows.Next() {
		var w domain.Webhook
		err := rows.Scan(&w.ID, &w.TeamID, &w.URL, &w.Events, &w.Secret, &w.Active, &w.CreatedAt)
		if err != nil {
			return nil, err
		}
		webhooks = append(webhooks, w)
	}
	return webhooks, rows.Err()
}
