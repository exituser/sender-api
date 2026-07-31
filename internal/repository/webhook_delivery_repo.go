package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sender-api/sender-api/internal/domain"
)

const maxWebhookDeliveryAttempts = 5

type WebhookDeliveryRepo struct {
	db *pgxpool.Pool
}

func NewWebhookDeliveryRepo(db *pgxpool.Pool) *WebhookDeliveryRepo {
	return &WebhookDeliveryRepo{db: db}
}

func (r *WebhookDeliveryRepo) CreateDelivery(ctx context.Context, delivery *domain.WebhookDelivery) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO webhook_deliveries (id, webhook_id, event_id, event, payload, status, attempts, next_attempt_at)
		VALUES ($1, $2, $3, $4, $5, 'pending', 0, NOW())
		ON CONFLICT (webhook_id, event_id) DO NOTHING
	`, delivery.ID, delivery.WebhookID, delivery.EventID, delivery.Event, delivery.Payload)
	return err
}

func (r *WebhookDeliveryRepo) ClaimDelivery(ctx context.Context) (*domain.WebhookDelivery, error) {
	var delivery domain.WebhookDelivery
	err := r.db.QueryRow(ctx, `
		WITH candidate AS (
			SELECT d.id, w.url, w.secret
			FROM webhook_deliveries d
			JOIN webhooks w ON w.id = d.webhook_id
			WHERE d.status = 'pending' AND d.next_attempt_at <= NOW()
			ORDER BY d.next_attempt_at, d.created_at
			FOR UPDATE OF d SKIP LOCKED
			LIMIT 1
		)
		UPDATE webhook_deliveries d
		SET status = 'sending',
			attempts = d.attempts + 1,
			lease_until = NOW() + INTERVAL '1 minute'
		FROM candidate c
		WHERE d.id = c.id
		RETURNING d.id, d.webhook_id, d.event_id, d.event, d.payload, d.status,
			d.attempts, d.next_attempt_at, d.lease_until, d.last_error, d.created_at,
			d.delivered_at, c.url, c.secret
	`).Scan(
		&delivery.ID, &delivery.WebhookID, &delivery.EventID, &delivery.Event, &delivery.Payload,
		&delivery.Status, &delivery.Attempts, &delivery.NextAttemptAt, &delivery.LeaseUntil,
		&delivery.LastError, &delivery.CreatedAt, &delivery.DeliveredAt, &delivery.URL, &delivery.Secret,
	)
	if err != nil {
		return nil, err
	}
	return &delivery, nil
}

func (r *WebhookDeliveryRepo) MarkDelivered(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE webhook_deliveries
		SET status = 'delivered', lease_until = NULL, delivered_at = NOW()
		WHERE id = $1 AND status = 'sending'
	`, id)
	return err
}

func (r *WebhookDeliveryRepo) MarkFailed(ctx context.Context, id uuid.UUID, reason string, retryAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE webhook_deliveries
		SET status = CASE WHEN attempts >= $2 THEN 'failed' ELSE 'pending' END,
			last_error = $3,
			next_attempt_at = $4,
			lease_until = NULL
		WHERE id = $1 AND status = 'sending'
	`, id, maxWebhookDeliveryAttempts, reason, retryAt)
	return err
}

func (r *WebhookDeliveryRepo) RecoverStale(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `
		UPDATE webhook_deliveries
		SET status = 'pending', lease_until = NULL
		WHERE status = 'sending' AND lease_until < NOW()
	`)
	if err != nil {
		return fmt.Errorf("recover stale webhook deliveries: %w", err)
	}
	return nil
}
