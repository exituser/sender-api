package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/pkg/metrics"
)

type DeliveryPipelineRepo struct {
	db *pgxpool.Pool
}

func NewDeliveryPipelineRepo(db *pgxpool.Pool) *DeliveryPipelineRepo {
	return &DeliveryPipelineRepo{db: db}
}

func (r *DeliveryPipelineRepo) CreateInboundWithOutbox(ctx context.Context, email *domain.InboundEmail, outbox *domain.WebhookOutboxEvent) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	toJSON, err := json.Marshal(email.To)
	if err != nil {
		return fmt.Errorf("marshal inbound recipients: %w", err)
	}
	attachments := email.Attachments
	if len(attachments) == 0 {
		attachments = json.RawMessage("[]")
	}
	headers := email.Headers
	if len(headers) == 0 {
		headers = json.RawMessage("{}")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO inbound_emails (id, team_id, message_id, from_addr, to_addr, subject, text, html, attachments, raw_s3_key, headers)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, email.ID, email.TeamID, email.MessageID, email.From, toJSON, email.Subject,
		email.Text, email.HTML, attachments, email.RawS3Key, headers); err != nil {
		return err
	}
	if err := insertOutbox(ctx, tx, outbox); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *DeliveryPipelineRepo) FinalizeAccepted(ctx context.Context, claim domain.SendAttemptClaim, providerMessageID string, event *domain.EmailEvent, outbox *domain.WebhookOutboxEvent) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE emails
		SET provider_message_id = $4,
			status = CASE WHEN status = 'sending' THEN 'sent' ELSE status END,
			send_attempt_state = 'accepted', send_lease_until = NULL,
			queue_recovery_pending = FALSE,
			sending_at = NULL, ambiguous_at = NULL, sent_at = COALESCE(sent_at, NOW())
		WHERE id = $1 AND send_attempt_id = $2 AND send_fence_token = $3
		  AND (
			(status = 'sending' AND send_attempt_state = 'send_started' AND send_lease_until >= NOW())
			OR (send_attempt_state = 'accepted' AND provider_message_id = $4)
		  )
	`, claim.EmailID, claim.AttemptID, claim.FenceToken, providerMessageID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() != 1 {
		metrics.AddCounter("sender_api_send_attempt_stale_fence_rejections_total", 1)
		return false, nil
	}
	if err := insertEmailEvent(ctx, tx, event); err != nil {
		return false, err
	}
	if err := insertOutbox(ctx, tx, outbox); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE provider_event_inbox
		SET status = CASE WHEN status = 'processing' AND lease_until >= NOW() THEN status ELSE 'pending' END,
			next_attempt_at = NOW(),
			lease_until = CASE WHEN status = 'processing' AND lease_until >= NOW() THEN lease_until ELSE NULL END
		WHERE status IN ('pending', 'processing')
		  AND (email_id = $1 OR send_attempt_id = $2 OR provider_message_id = $3)
	`, claim.EmailID, claim.AttemptID, providerMessageID); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *DeliveryPipelineRepo) FinalizeFailed(ctx context.Context, claim domain.SendAttemptClaim, event *domain.EmailEvent, outbox *domain.WebhookOutboxEvent) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE emails
		SET status = 'failed', send_attempt_state = 'failed_terminal',
			send_fence_token = NULL, send_lease_until = NULL,
			queue_recovery_pending = FALSE, sending_at = NULL, ambiguous_at = NULL
		WHERE id = $1 AND send_attempt_id = $2 AND send_fence_token = $3
		  AND status = 'sending' AND send_attempt_state IN ('leased', 'send_started')
	`, claim.EmailID, claim.AttemptID, claim.FenceToken)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() != 1 {
		metrics.AddCounter("sender_api_send_attempt_stale_fence_rejections_total", 1)
		return false, nil
	}
	if err := insertEmailEvent(ctx, tx, event); err != nil {
		return false, err
	}
	if err := insertOutbox(ctx, tx, outbox); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *DeliveryPipelineRepo) FinalizeAmbiguous(ctx context.Context, claim domain.SendAttemptClaim, event *domain.EmailEvent, outbox *domain.WebhookOutboxEvent) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE emails
		SET status = 'ambiguous', send_attempt_state = 'ambiguous',
			send_fence_token = NULL, send_lease_until = NULL,
			queue_recovery_pending = FALSE, sending_at = NULL,
			ambiguous_at = COALESCE(ambiguous_at, NOW())
		WHERE id = $1 AND send_attempt_id = $2 AND send_fence_token = $3
		  AND status = 'sending' AND send_attempt_state = 'send_started'
	`, claim.EmailID, claim.AttemptID, claim.FenceToken)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() != 1 {
		metrics.AddCounter("sender_api_send_attempt_stale_fence_rejections_total", 1)
		return false, nil
	}
	if err := insertEmailEvent(ctx, tx, event); err != nil {
		return false, err
	}
	if err := insertOutbox(ctx, tx, outbox); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	metrics.AddCounter("sender_api_send_attempt_ambiguous_total", 1)
	return true, nil
}

func (r *DeliveryPipelineRepo) ReconcileAmbiguous(ctx context.Context, teamID, emailID uuid.UUID, action, providerMessageID string, event *domain.EmailEvent, outbox *domain.WebhookOutboxEvent) (bool, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	providerMessageID = strings.TrimSpace(providerMessageID)
	if action != "accepted" && action != "failed" {
		return false, fmt.Errorf("reconciliation action must be accepted or failed")
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var attemptID *uuid.UUID
	var existingProviderID *string
	if err := tx.QueryRow(ctx, `
		SELECT send_attempt_id, provider_message_id
		FROM emails
		WHERE id = $1 AND team_id = $2
		  AND status = 'ambiguous' AND send_attempt_state = 'ambiguous'
		FOR UPDATE
	`, emailID, teamID).Scan(&attemptID, &existingProviderID); err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	if action == "accepted" {
		if providerMessageID == "" && existingProviderID != nil {
			providerMessageID = strings.TrimSpace(*existingProviderID)
		}
		if providerMessageID == "" {
			var candidate *string
			if err := tx.QueryRow(ctx, `
				SELECT CASE WHEN COUNT(DISTINCT provider_message_id) = 1
					THEN MIN(provider_message_id) ELSE NULL END
					FROM provider_event_inbox
					WHERE status <> 'ignored'
					  AND lower(replace(btrim(event_type), ' ', '_')) IN ('send', 'delivery')
					  AND (email_id = $1 OR ($2::uuid IS NOT NULL AND send_attempt_id = $2))
			`, emailID, attemptID).Scan(&candidate); err != nil {
				return false, err
			}
			if candidate != nil {
				providerMessageID = strings.TrimSpace(*candidate)
			}
		}
		if providerMessageID == "" {
			return false, domain.ErrDeliveryConfirmationUnavailable
		}

		var verified bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM provider_event_inbox
				WHERE provider_message_id = $3 AND status <> 'ignored'
				  AND lower(replace(btrim(event_type), ' ', '_')) IN ('send', 'delivery')
				  AND (email_id = $1 OR ($2::uuid IS NOT NULL AND send_attempt_id = $2))
			)
		`, emailID, attemptID, providerMessageID).Scan(&verified); err != nil {
			return false, err
		}
		if !verified {
			return false, domain.ErrDeliveryConfirmationUnavailable
		}
		if _, err := tx.Exec(ctx, `
			UPDATE emails
			SET provider_message_id = $2, status = 'sent', send_attempt_state = 'accepted',
				send_fence_token = NULL, send_lease_until = NULL,
				queue_recovery_pending = FALSE, sending_at = NULL, ambiguous_at = NULL,
				sent_at = COALESCE(sent_at, NOW())
			WHERE id = $1
		`, emailID, providerMessageID); err != nil {
			return false, err
		}
	} else if _, err := tx.Exec(ctx, `
		UPDATE emails
		SET status = 'failed', send_attempt_state = 'failed_terminal',
			send_fence_token = NULL, send_lease_until = NULL,
			queue_recovery_pending = FALSE, sending_at = NULL, ambiguous_at = NULL
		WHERE id = $1
	`, emailID); err != nil {
		return false, err
	}

	if err := insertEmailEvent(ctx, tx, event); err != nil {
		return false, err
	}
	if err := insertOutbox(ctx, tx, outbox); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *DeliveryPipelineRepo) StoreProviderEvent(ctx context.Context, event *domain.ProviderEventInbox) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO provider_event_inbox (event_id, provider_message_id, event_type, payload, email_id, send_attempt_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (event_id) DO NOTHING
	`, event.EventID, event.ProviderMessageID, event.EventType, event.Payload, event.EmailID, event.SendAttemptID)
	return err
}

func (r *DeliveryPipelineRepo) ClaimProviderEvent(ctx context.Context, eventID *uuid.UUID) (*domain.ProviderEventInbox, error) {
	var event domain.ProviderEventInbox
	err := r.db.QueryRow(ctx, `
		WITH candidate AS (
			SELECT event_id
			FROM provider_event_inbox
			WHERE status = 'pending' AND next_attempt_at <= NOW() AND payload IS NOT NULL
			  AND ($1::uuid IS NULL OR event_id = $1)
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE provider_event_inbox i
		SET status = 'processing', attempts = attempts + 1,
			lease_until = NOW() + INTERVAL '1 minute'
		FROM candidate c
		WHERE i.event_id = c.event_id
		RETURNING i.event_id, i.provider_message_id, i.event_type, i.payload,
			i.email_id, i.send_attempt_id, i.attempts, i.created_at
	`, eventID).Scan(&event.EventID, &event.ProviderMessageID, &event.EventType, &event.Payload,
		&event.EmailID, &event.SendAttemptID, &event.Attempts, &event.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (r *DeliveryPipelineRepo) RetryProviderEvent(ctx context.Context, eventID uuid.UUID, reason string, retryAt time.Time, terminal bool) error {
	status := "pending"
	if terminal {
		status = "ignored"
	}
	_, err := r.db.Exec(ctx, `
		UPDATE provider_event_inbox
		SET status = $2, next_attempt_at = $3, lease_until = NULL,
			last_error = $4, processed_at = CASE WHEN $2 = 'ignored' THEN NOW() ELSE NULL END
		WHERE event_id = $1 AND status = 'processing'
	`, eventID, status, retryAt, reason)
	return err
}

func (r *DeliveryPipelineRepo) ApplyProviderEvent(ctx context.Context, inboxEventID, emailID uuid.UUID, providerMessageID string, targetStatus domain.EmailStatus, event *domain.EmailEvent, outbox *domain.WebhookOutboxEvent, suppressions []domain.Suppression) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentStatus domain.EmailStatus
	var currentProviderID *string
	if err := tx.QueryRow(ctx, `
		SELECT status, provider_message_id FROM emails WHERE id = $1 FOR UPDATE
	`, emailID).Scan(&currentStatus, &currentProviderID); err != nil {
		return err
	}
	if currentProviderID != nil && *currentProviderID != providerMessageID {
		return domain.ErrProviderCorrelationMismatch
	}
	applyStatus := domain.ShouldApplyProviderStatus(currentStatus, targetStatus)
	if _, err := tx.Exec(ctx, `
		UPDATE emails
		SET provider_message_id = COALESCE(provider_message_id, $2),
			status = CASE WHEN $3 THEN $4 ELSE status END,
			send_attempt_state = CASE
				WHEN $3 AND $4 = 'failed' THEN 'failed_terminal'
				WHEN $3 AND $4 <> '' THEN 'accepted'
				ELSE send_attempt_state
			END,
			send_lease_until = CASE WHEN $3 AND $4 <> '' THEN NULL ELSE send_lease_until END,
			send_fence_token = CASE WHEN $3 AND $4 <> '' THEN NULL ELSE send_fence_token END,
			queue_recovery_pending = CASE WHEN $3 AND $4 <> '' THEN FALSE ELSE queue_recovery_pending END,
			sending_at = CASE WHEN $3 AND $4 <> '' THEN NULL ELSE sending_at END,
			ambiguous_at = CASE WHEN $3 AND $4 <> '' THEN NULL ELSE ambiguous_at END,
			sent_at = CASE WHEN $3 AND $4 <> '' AND $4 <> 'failed' THEN COALESCE(sent_at, NOW()) ELSE sent_at END
		WHERE id = $1
	`, emailID, providerMessageID, applyStatus, targetStatus); err != nil {
		return err
	}
	for index := range suppressions {
		suppression := suppressions[index]
		if _, err := tx.Exec(ctx, `
			INSERT INTO suppressions (id, team_id, email, reason)
			VALUES ($1, $2, $3, $4)
				ON CONFLICT (team_id, email) DO UPDATE
				SET reason = CASE
					WHEN suppressions.reason = 'complaint' OR EXCLUDED.reason = 'complaint' THEN 'complaint'
					WHEN suppressions.reason = 'bounce' OR EXCLUDED.reason = 'bounce' THEN 'bounce'
					ELSE 'unsubscribe'
				END,
				updated_at = NOW()
		`, suppression.ID, suppression.TeamID, domain.NormalizeEmail(suppression.Email), suppression.Reason); err != nil {
			return err
		}
	}
	if err := insertEmailEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := insertOutbox(ctx, tx, outbox); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE provider_event_inbox
		SET status = 'processed', email_id = $2, lease_until = NULL,
			last_error = NULL, processed_at = NOW()
		WHERE event_id = $1 AND status = 'processing'
	`, inboxEventID, emailID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	metrics.AddCounter("sender_api_provider_events_processed_total", 1)
	return nil
}

func (r *DeliveryPipelineRepo) DispatchNextOutbox(ctx context.Context) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var item domain.WebhookOutboxEvent
	err = tx.QueryRow(ctx, `
		SELECT id, team_id, event_id, event, payload, retention_class, created_at
		FROM webhook_outbox
		WHERE status = 'pending' AND next_attempt_at <= NOW() AND payload IS NOT NULL
		ORDER BY next_attempt_at, created_at
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`).Scan(&item.ID, &item.TeamID, &item.EventID, &item.Event, &item.Payload, &item.RetentionClass, &item.CreatedAt)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE webhook_outbox
		SET status = 'processing', attempts = attempts + 1,
			lease_until = NOW() + INTERVAL '1 minute'
		WHERE id = $1
	`, item.ID); err != nil {
		return false, err
	}

	rows, err := tx.Query(ctx, `
		SELECT id FROM webhooks
		WHERE team_id = $1 AND active = true AND $2 = ANY(events)
	`, item.TeamID, item.Event)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	webhookIDs := make([]uuid.UUID, 0)
	for rows.Next() {
		var webhookID uuid.UUID
		if err := rows.Scan(&webhookID); err != nil {
			return false, err
		}
		webhookIDs = append(webhookIDs, webhookID)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	rows.Close()
	for _, webhookID := range webhookIDs {
		deliveryID := uuid.NewSHA1(item.EventID, []byte(webhookID.String()))
		if _, err := tx.Exec(ctx, `
			INSERT INTO webhook_deliveries
				(id, webhook_id, event_id, event, payload, retention_class, status, attempts, next_attempt_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'pending', 0, NOW())
			ON CONFLICT (webhook_id, event_id) DO NOTHING
		`, deliveryID, webhookID, item.EventID, item.Event, item.Payload, item.RetentionClass); err != nil {
			return false, err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE webhook_outbox
		SET status = 'dispatched', lease_until = NULL, dispatched_at = NOW(), last_error = NULL
		WHERE id = $1 AND status = 'processing'
	`, item.ID); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	metrics.AddCounter("sender_api_webhook_outbox_dispatched_total", 1)
	return true, nil
}

func (r *DeliveryPipelineRepo) RecoverStalePipelineWork(ctx context.Context) error {
	providerTag, err := r.db.Exec(ctx, `
		UPDATE provider_event_inbox
		SET status = 'pending', lease_until = NULL
		WHERE status = 'processing' AND lease_until < NOW()
	`)
	if err != nil {
		return fmt.Errorf("recover provider event inbox: %w", err)
	}
	outboxTag, err := r.db.Exec(ctx, `
		UPDATE webhook_outbox
		SET status = 'pending', lease_until = NULL
		WHERE status = 'processing' AND lease_until < NOW()
	`)
	if err != nil {
		return fmt.Errorf("recover webhook outbox: %w", err)
	}
	metrics.AddCounter("sender_api_provider_events_lease_recovered_total", uint64(providerTag.RowsAffected()))
	metrics.AddCounter("sender_api_webhook_outbox_lease_recovered_total", uint64(outboxTag.RowsAffected()))
	return nil
}

func insertEmailEvent(ctx context.Context, tx pgx.Tx, event *domain.EmailEvent) error {
	if event == nil {
		return nil
	}
	data := event.Data
	if len(data) == 0 {
		data = json.RawMessage("{}")
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO email_events (id, email_id, event, data, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO NOTHING
	`, event.ID, event.EmailID, event.Event, data, nullableString(event.IPAddress), nullableString(event.UserAgent))
	return err
}

func insertOutbox(ctx context.Context, tx pgx.Tx, outbox *domain.WebhookOutboxEvent) error {
	if outbox == nil {
		return nil
	}
	payload := outbox.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO webhook_outbox (id, team_id, event_id, event, payload, retention_class)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (event_id) DO NOTHING
	`, outbox.ID, outbox.TeamID, outbox.EventID, outbox.Event, payload, outbox.RetentionClass)
	return err
}
