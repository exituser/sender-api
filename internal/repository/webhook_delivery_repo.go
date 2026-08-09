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
		INSERT INTO webhook_deliveries (id, webhook_id, event_id, event, payload, retention_class, status, attempts, next_attempt_at)
		VALUES ($1, $2, $3, $4, $5,
			CASE WHEN $4 LIKE 'inbound.%' THEN 'inbound' ELSE 'outbound' END,
			'pending', 0, NOW())
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
			WHERE w.active = true AND d.status = 'pending' AND d.payload IS NOT NULL
			  AND d.next_attempt_at <= NOW()
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

func (r *WebhookDeliveryRepo) ReplayFailed(ctx context.Context, teamID, webhookID, deliveryID uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE webhook_deliveries d
		SET status = 'pending', attempts = 0, next_attempt_at = NOW(),
			lease_until = NULL, last_error = NULL, delivered_at = NULL
		FROM webhooks w
		WHERE d.id = $1 AND d.webhook_id = $2 AND w.id = d.webhook_id
		  AND w.team_id = $3 AND w.active = true AND d.status = 'failed'
		  AND d.payload IS NOT NULL
	`, deliveryID, webhookID, teamID)
	if err != nil {
		return fmt.Errorf("replay webhook delivery: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("failed webhook delivery not found")
	}
	return nil
}

// PurgeByEventClass removes payloads older than before for one webhook event
// class (for example, outbound or inbound), while retaining delivery metadata.
func (r *WebhookDeliveryRepo) PurgeByEventClass(ctx context.Context, eventClass string, before time.Time) (int64, error) {
	if eventClass != "outbound" && eventClass != "inbound" {
		return 0, fmt.Errorf("unsupported webhook event class %q", eventClass)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var purged int64
	tag, err := tx.Exec(ctx, `
		UPDATE webhook_deliveries
		SET payload = NULL,
			status = CASE WHEN status IN ('pending', 'sending') THEN 'failed' ELSE status END,
			lease_until = NULL,
			last_error = CASE WHEN status IN ('pending', 'sending')
				THEN 'payload removed by retention policy' ELSE last_error END
		WHERE created_at < $1 AND retention_class = $2 AND payload IS NOT NULL
	`, before, eventClass)
	if err != nil {
		return 0, fmt.Errorf("purge %s webhook payloads: %w", eventClass, err)
	}
	purged += tag.RowsAffected()

	tag, err = tx.Exec(ctx, `
		UPDATE webhook_outbox
		SET payload = NULL,
			status = CASE WHEN status IN ('pending', 'processing') THEN 'failed' ELSE status END,
			lease_until = NULL,
			last_error = CASE WHEN status IN ('pending', 'processing')
				THEN 'payload removed by retention policy' ELSE last_error END
		WHERE created_at < $1 AND retention_class = $2 AND payload IS NOT NULL
	`, before, eventClass)
	if err != nil {
		return 0, fmt.Errorf("purge %s webhook outbox payloads: %w", eventClass, err)
	}
	purged += tag.RowsAffected()

	if eventClass == "outbound" {
		tag, err = tx.Exec(ctx, `
			UPDATE provider_event_inbox
			SET payload = NULL,
				status = CASE WHEN status IN ('pending', 'processing') THEN 'ignored' ELSE status END,
				lease_until = NULL,
				last_error = CASE WHEN status IN ('pending', 'processing')
					THEN 'payload removed by retention policy' ELSE last_error END,
				processed_at = CASE WHEN status IN ('pending', 'processing')
					THEN COALESCE(processed_at, NOW()) ELSE processed_at END
			WHERE created_at < $1 AND payload IS NOT NULL
		`, before)
		if err != nil {
			return 0, fmt.Errorf("purge provider event inbox payloads: %w", err)
		}
		purged += tag.RowsAffected()
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return purged, nil
}

// ListForWebhook returns delivery attempts for a webhook owned by teamID.
// Joining webhooks here keeps the tenant boundary in the query instead of
// relying on the handler to perform a second, race-prone ownership check.
func (r *WebhookDeliveryRepo) ListForWebhook(ctx context.Context, teamID, webhookID uuid.UUID, limit int) ([]domain.WebhookDelivery, error) {
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
		SELECT d.id, d.webhook_id, d.event_id, d.event, d.payload, d.status,
			d.attempts, d.next_attempt_at, d.last_error, d.created_at, d.delivered_at
		FROM webhook_deliveries d
		JOIN webhooks w ON w.id = d.webhook_id
		WHERE w.team_id = $1 AND d.webhook_id = $2
		ORDER BY d.created_at DESC
		LIMIT $3
	`, teamID, webhookID, limit)
	if err != nil {
		return nil, fmt.Errorf("list webhook deliveries: %w", err)
	}
	defer rows.Close()

	deliveries := make([]domain.WebhookDelivery, 0)
	for rows.Next() {
		var delivery domain.WebhookDelivery
		if err := rows.Scan(
			&delivery.ID, &delivery.WebhookID, &delivery.EventID, &delivery.Event,
			&delivery.Payload, &delivery.Status, &delivery.Attempts, &delivery.NextAttemptAt,
			&delivery.LastError, &delivery.CreatedAt, &delivery.DeliveredAt,
		); err != nil {
			return nil, fmt.Errorf("scan webhook delivery: %w", err)
		}
		deliveries = append(deliveries, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate webhook deliveries: %w", err)
	}
	return deliveries, nil
}
