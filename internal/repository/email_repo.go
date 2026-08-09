package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/pkg/metrics"
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
	category := email.Category
	if category == "" {
		category = domain.EmailCategoryTransactional
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO emails (id, team_id, api_key_id, from_addr, to_addr, cc, bcc, subject, category, html, text, reply_to, attachments, status, tags, metadata, headers, idempotency_key, idempotency_hash, scheduled_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
	`,
		email.ID, email.TeamID, email.APIKeyID, email.From,
		toJSON, ccJSON, bccJSON,
		email.Subject, category, email.HTML, email.Text, replyToJSON, attachmentsJSON, email.Status,
		tagsJSON, metadataJSON, headersJSON, email.IdempotencyKey, email.IdempotencyHash, email.ScheduledAt,
	)
	return err
}

func (r *EmailRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Email, error) {
	var email domain.Email
	var toJSON, ccJSON, bccJSON, replyToJSON, attachmentsJSON, tagsJSON, metadataJSON, headersJSON []byte

	err := r.db.QueryRow(ctx, `
		SELECT id, team_id, api_key_id, from_addr, to_addr, cc, bcc, subject, category, html, text, reply_to, attachments, status, tags, metadata, headers, idempotency_key, idempotency_hash, provider_message_id,
			send_attempt_id, send_fence_token, send_attempt_state, send_lease_until, sending_at, ambiguous_at, scheduled_at, sent_at, created_at,
				EXISTS (
					SELECT 1 FROM provider_event_inbox i
					WHERE i.status <> 'ignored'
					  AND lower(replace(btrim(i.event_type), ' ', '_')) IN ('send', 'delivery')
					  AND (
					i.email_id = emails.id
					OR (emails.send_attempt_id IS NOT NULL AND i.send_attempt_id = emails.send_attempt_id)
					OR (emails.provider_message_id IS NOT NULL AND i.provider_message_id = emails.provider_message_id)
				)
			) AS provider_evidence
		FROM emails WHERE id = $1
	`, id).Scan(
		&email.ID, &email.TeamID, &email.APIKeyID, &email.From,
		&toJSON, &ccJSON, &bccJSON,
		&email.Subject, &email.Category, &email.HTML, &email.Text, &replyToJSON, &attachmentsJSON, &email.Status,
		&tagsJSON, &metadataJSON, &headersJSON, &email.IdempotencyKey, &email.IdempotencyHash, &email.ProviderMessageID,
		&email.SendAttemptID, &email.SendFenceToken, &email.SendAttemptState, &email.SendLeaseUntil, &email.SendingAt,
		&email.AmbiguousAt, &email.ScheduledAt, &email.SentAt, &email.CreatedAt, &email.ProviderEvidence,
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
	var email domain.Email
	var toJSON, ccJSON, bccJSON, replyToJSON, attachmentsJSON, tagsJSON, metadataJSON, headersJSON []byte
	err := r.db.QueryRow(ctx, `
		SELECT id, team_id, api_key_id, from_addr, to_addr, cc, bcc, subject, category, html, text, reply_to, attachments, status, tags, metadata, headers, idempotency_key, idempotency_hash, provider_message_id,
			send_attempt_id, send_fence_token, send_attempt_state, send_lease_until, sending_at, ambiguous_at, scheduled_at, sent_at, created_at,
				EXISTS (
					SELECT 1 FROM provider_event_inbox i
					WHERE i.status <> 'ignored'
					  AND lower(replace(btrim(i.event_type), ' ', '_')) IN ('send', 'delivery')
					  AND (
					i.email_id = emails.id
					OR (emails.send_attempt_id IS NOT NULL AND i.send_attempt_id = emails.send_attempt_id)
					OR (emails.provider_message_id IS NOT NULL AND i.provider_message_id = emails.provider_message_id)
				)
			) AS provider_evidence
		FROM emails WHERE id = $1 AND team_id = $2
	`, id, teamID).Scan(
		&email.ID, &email.TeamID, &email.APIKeyID, &email.From,
		&toJSON, &ccJSON, &bccJSON,
		&email.Subject, &email.Category, &email.HTML, &email.Text, &replyToJSON, &attachmentsJSON, &email.Status,
		&tagsJSON, &metadataJSON, &headersJSON, &email.IdempotencyKey, &email.IdempotencyHash, &email.ProviderMessageID,
		&email.SendAttemptID, &email.SendFenceToken, &email.SendAttemptState, &email.SendLeaseUntil, &email.SendingAt,
		&email.AmbiguousAt, &email.ScheduledAt, &email.SentAt, &email.CreatedAt, &email.ProviderEvidence,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("email not found: %w", err)
		}
		return nil, fmt.Errorf("get team email: %w", err)
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

// MarkProviderAccepted records the provider receipt and terminal sent state in
// one database statement. Once SES has accepted a message, a later worker
// retry must never submit it again just because one local write failed.
func (r *EmailRepo) MarkProviderAccepted(ctx context.Context, id uuid.UUID, messageID string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE emails
		SET provider_message_id = $1,
			status = CASE WHEN status IN ('sending', 'ambiguous', 'failed') THEN 'sent' ELSE status END,
			send_attempt_state = 'accepted', send_lease_until = NULL,
			send_fence_token = NULL, queue_recovery_pending = FALSE,
			sending_at = NULL, ambiguous_at = NULL, sent_at = COALESCE(sent_at, NOW())
		WHERE id = $2 AND status IN ('sending', 'sent', 'delivered', 'opened', 'clicked', 'bounced', 'complained')
	`, messageID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("email %s was not in sending state", id)
	}
	return nil
}

func (r *EmailRepo) ClaimSendAttempt(ctx context.Context, claim domain.SendAttemptClaim) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE emails
		SET status = 'sending', sending_at = NOW(), send_attempt_id = $2,
			send_fence_token = $3, send_attempt_state = 'leased', send_lease_until = $4,
			ambiguous_at = NULL, queue_recovery_pending = FALSE
		WHERE id = $1 AND status = 'queued' AND send_attempt_state IN ('none', 'failed_terminal')
	`, claim.EmailID, claim.AttemptID, claim.FenceToken, claim.LeaseUntil)
	return tag.RowsAffected() == 1, err
}

func (r *EmailRepo) MarkSendStarted(ctx context.Context, claim domain.SendAttemptClaim) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE emails
		SET send_attempt_state = 'send_started', send_lease_until = $4
		WHERE id = $1 AND send_attempt_id = $2 AND send_fence_token = $3
		  AND status = 'sending' AND send_attempt_state = 'leased'
	`, claim.EmailID, claim.AttemptID, claim.FenceToken, claim.LeaseUntil)
	return tag.RowsAffected() == 1, err
}

func (r *EmailRepo) MarkSendRetryable(ctx context.Context, claim domain.SendAttemptClaim) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE emails
		SET status = 'queued', sending_at = NULL, send_attempt_state = 'none',
			send_attempt_id = NULL, send_fence_token = NULL, send_lease_until = NULL,
			ambiguous_at = NULL, queue_recovery_pending = FALSE
		WHERE id = $1 AND send_attempt_id = $2 AND send_fence_token = $3
		  AND status = 'sending' AND send_attempt_state IN ('leased', 'send_started')
	`, claim.EmailID, claim.AttemptID, claim.FenceToken)
	return tag.RowsAffected() == 1, err
}

func (r *EmailRepo) MarkSendAmbiguous(ctx context.Context, claim domain.SendAttemptClaim, _ string) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE emails
		SET status = 'ambiguous', sending_at = NULL, send_attempt_state = 'ambiguous',
			send_fence_token = NULL, send_lease_until = NULL,
			queue_recovery_pending = FALSE, ambiguous_at = COALESCE(ambiguous_at, NOW())
		WHERE id = $1 AND send_attempt_id = $2 AND send_fence_token = $3
		  AND status = 'sending' AND send_attempt_state = 'send_started'
	`, claim.EmailID, claim.AttemptID, claim.FenceToken)
	return tag.RowsAffected() == 1, err
}

func (r *EmailRepo) RecoverExpiredSendAttempts(ctx context.Context) ([]string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		UPDATE emails
		SET status = 'queued', sending_at = NULL, send_attempt_state = 'none',
			send_attempt_id = NULL, send_fence_token = NULL, send_lease_until = NULL,
			ambiguous_at = NULL, queue_recovery_pending = TRUE
		WHERE status = 'sending' AND send_attempt_state = 'leased'
		  AND send_lease_until IS NOT NULL AND send_lease_until < NOW()
		RETURNING id::text
	`)
	if err != nil {
		return nil, err
	}
	var recovered []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		recovered = append(recovered, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	ambiguousTag, err := tx.Exec(ctx, `
		UPDATE emails
		SET status = 'ambiguous', sending_at = NULL,
			send_attempt_state = 'ambiguous', send_fence_token = NULL,
			send_lease_until = NULL, queue_recovery_pending = FALSE,
			ambiguous_at = COALESCE(ambiguous_at, NOW())
		WHERE status = 'sending'
		  AND (
			send_attempt_state = 'send_started'
			OR (send_attempt_state = 'none' AND provider_message_id IS NULL)
		  )
		  AND (send_lease_until < NOW() OR (send_lease_until IS NULL AND sending_at < NOW() - INTERVAL '10 minutes'))
	`)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	metrics.AddCounter("sender_api_send_attempt_leases_recovered_total", uint64(len(recovered)))
	metrics.AddCounter("sender_api_send_attempt_ambiguous_total", uint64(ambiguousTag.RowsAffected()))
	r.observeSendAttemptMetrics(ctx)
	return recovered, nil
}

func (r *EmailRepo) ListQueueRecoveryPending(ctx context.Context, limit int) ([]string, error) {
	if limit < 1 {
		limit = 1000
	}
	if limit > 5000 {
		limit = 5000
	}
	rows, err := r.db.Query(ctx, `
		SELECT id::text
		FROM emails
		WHERE status = 'queued' AND queue_recovery_pending = TRUE
		ORDER BY created_at, id
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *EmailRepo) MarkQueueRecoveryEnqueued(ctx context.Context, emailID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `
		UPDATE emails SET queue_recovery_pending = FALSE
		WHERE id = $1 AND queue_recovery_pending = TRUE
	`, emailID)
	return err
}

// MarkDeadLetterFailed closes any provider-attempt ownership before the Redis
// receipt is discarded. A later manual replay must start from a clean attempt
// instead of inheriting a stale fence or lease.
func (r *EmailRepo) MarkDeadLetterFailed(ctx context.Context, emailID uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE emails
		SET status = 'failed', sending_at = NULL,
			send_attempt_state = 'failed_terminal', send_attempt_id = NULL,
			send_fence_token = NULL, send_lease_until = NULL,
			ambiguous_at = NULL, queue_recovery_pending = FALSE
		WHERE id = $1
		  AND (
			(status = 'queued' AND send_attempt_state = 'none'
			 AND send_attempt_id IS NULL AND send_fence_token IS NULL
			 AND queue_recovery_pending = FALSE)
			OR (status = 'failed' AND send_attempt_state = 'failed_terminal'
			 AND send_attempt_id IS NULL AND send_fence_token IS NULL)
		  )
	`, emailID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("email %s is not eligible for dead-letter failure", emailID)
	}
	return nil
}

func (r *EmailRepo) PrepareDeadLetterReplay(ctx context.Context, teamID, emailID, replayToken uuid.UUID) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE emails
		SET status = 'queued', sending_at = NULL,
			send_attempt_state = 'none', send_attempt_id = $3,
			send_fence_token = NULL, send_lease_until = NULL,
			ambiguous_at = NULL, queue_recovery_pending = FALSE
		WHERE id = $1 AND team_id = $2
		  AND status = 'failed'
		  AND send_attempt_state IN ('none', 'failed_terminal')
	`, emailID, teamID, replayToken)
	return tag.RowsAffected() == 1, err
}

func (r *EmailRepo) CancelDeadLetterReplay(ctx context.Context, emailID, replayToken uuid.UUID) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE emails
		SET status = 'failed', sending_at = NULL,
			send_attempt_state = 'failed_terminal', send_attempt_id = NULL,
			send_fence_token = NULL, send_lease_until = NULL,
			ambiguous_at = NULL, queue_recovery_pending = FALSE
		WHERE id = $1 AND status = 'queued' AND send_attempt_state = 'none'
		  AND send_attempt_id = $2
	`, emailID, replayToken)
	return tag.RowsAffected() == 1, err
}

func (r *EmailRepo) observeSendAttemptMetrics(ctx context.Context) {
	states := []domain.SendAttemptState{
		domain.SendAttemptNone,
		domain.SendAttemptLeased,
		domain.SendAttemptStarted,
		domain.SendAttemptAccepted,
		domain.SendAttemptAmbiguous,
		domain.SendAttemptFailedTerminal,
	}
	for _, state := range states {
		metrics.SetGauge("sender_api_send_attempt_"+string(state), 0)
	}
	rows, err := r.db.Query(ctx, `
		SELECT send_attempt_state, COUNT(*)::bigint
		FROM emails
		GROUP BY send_attempt_state
	`)
	if err == nil {
		for rows.Next() {
			var state domain.SendAttemptState
			var count int64
			if rows.Scan(&state, &count) == nil {
				metrics.SetGauge("sender_api_send_attempt_"+string(state), count)
			}
		}
		rows.Close()
	}
	var oldestSeconds int64
	if err := r.db.QueryRow(ctx, `
		SELECT COALESCE(EXTRACT(EPOCH FROM NOW() - MIN(ambiguous_at))::bigint, 0)
		FROM emails WHERE send_attempt_state = 'ambiguous'
	`).Scan(&oldestSeconds); err == nil {
		metrics.SetGauge("sender_api_send_attempt_ambiguous_oldest_seconds", oldestSeconds)
	}
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
		UPDATE emails SET status = 'cancelled', queue_recovery_pending = FALSE
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
		SELECT id, team_id, api_key_id, from_addr, to_addr, subject, category, status, send_attempt_state, created_at
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
		err := rows.Scan(&email.ID, &email.TeamID, &email.APIKeyID, &email.From, &toJSON, &email.Subject, &email.Category, &email.Status, &email.SendAttemptState, &email.CreatedAt)
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
	_, err := r.RecoverExpiredSendAttempts(ctx)
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

func (r *EmailRepo) PurgeEmailsBefore(ctx context.Context, before time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM emails
		WHERE created_at < $1
		  AND status IN ('sent', 'delivered', 'opened', 'clicked', 'bounced', 'complained', 'failed', 'cancelled', 'ambiguous')
	`, before)
	if err != nil {
		return 0, fmt.Errorf("purge expired emails: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *EmailRepo) AddEvent(ctx context.Context, event *domain.EmailEvent) error {
	dataJSON, _ := json.Marshal(event.Data)
	_, err := r.db.Exec(ctx, `
		INSERT INTO email_events (id, email_id, event, data, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, event.ID, event.EmailID, event.Event, dataJSON, nullableString(event.IPAddress), nullableString(event.UserAgent))
	return err
}

func (r *EmailRepo) AddEventWithDeliveries(ctx context.Context, event *domain.EmailEvent, deliveries []*domain.WebhookDelivery) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	dataJSON, _ := json.Marshal(event.Data)
	if _, err := tx.Exec(ctx, `
		INSERT INTO email_events (id, email_id, event, data, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, event.ID, event.EmailID, event.Event, dataJSON, nullableString(event.IPAddress), nullableString(event.UserAgent)); err != nil {
		return err
	}
	for _, delivery := range deliveries {
		if _, err := tx.Exec(ctx, `
			INSERT INTO webhook_deliveries (id, webhook_id, event_id, event, payload, retention_class, status, attempts, next_attempt_at)
			VALUES ($1, $2, $3, $4, $5,
				CASE WHEN $4 LIKE 'inbound.%' THEN 'inbound' ELSE 'outbound' END,
				'pending', 0, NOW())
			ON CONFLICT (webhook_id, event_id) DO NOTHING
		`, delivery.ID, delivery.WebhookID, delivery.EventID, delivery.Event, delivery.Payload); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
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
		var ipAddress, userAgent *string
		err := rows.Scan(&event.ID, &event.EmailID, &event.Event, &event.Timestamp, &dataJSON, &ipAddress, &userAgent)
		if err != nil {
			return nil, err
		}
		if ipAddress != nil {
			event.IPAddress = *ipAddress
		}
		if userAgent != nil {
			event.UserAgent = *userAgent
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
		var ipAddress, userAgent *string
		if err := rows.Scan(&event.ID, &event.EmailID, &event.Event, &event.Timestamp, &dataJSON, &ipAddress, &userAgent); err != nil {
			return nil, err
		}
		if ipAddress != nil {
			event.IPAddress = *ipAddress
		}
		if userAgent != nil {
			event.UserAgent = *userAgent
		}
		_ = json.Unmarshal(dataJSON, &event.Data)
		events = append(events, event)
	}
	return events, rows.Err()
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
