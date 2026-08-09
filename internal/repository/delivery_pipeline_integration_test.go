//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sender-api/sender-api/internal/domain"
)

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN is not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect integration database: %v", err)
	}
	if err := CheckSchemaVersion(context.Background(), pool); err != nil {
		pool.Close()
		t.Fatalf("integration schema: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func integrationTeam(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO teams (id, name, slug) VALUES ($1, $2, $3)
	`, id, "integration", "integration-"+id.String()); err != nil {
		t.Fatalf("create integration team: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM teams WHERE id = $1`, id) })
	return id
}

func integrationEmail(t *testing.T, pool *pgxpool.Pool, teamID uuid.UUID) *domain.Email {
	t.Helper()
	email := &domain.Email{
		ID: uuid.New(), TeamID: teamID, From: "sender@example.com",
		To: []string{"person@example.net"}, Subject: "Integration", Text: "Body",
		Category: domain.EmailCategoryTransactional, Status: domain.EmailStatusQueued,
	}
	if err := NewEmailRepo(pool).Create(context.Background(), email); err != nil {
		t.Fatalf("create integration email: %v", err)
	}
	return email
}

func TestSendAttemptFencingAndAmbiguousRecoveryIntegration(t *testing.T) {
	pool := integrationPool(t)
	teamID := integrationTeam(t, pool)
	email := integrationEmail(t, pool, teamID)
	repo := NewEmailRepo(pool)

	claim := domain.SendAttemptClaim{
		EmailID: email.ID, AttemptID: uuid.New(), FenceToken: uuid.New(),
		LeaseUntil: time.Now().UTC().Add(-time.Minute),
	}
	claimed, err := repo.ClaimSendAttempt(context.Background(), claim)
	if err != nil || !claimed {
		t.Fatalf("claim send attempt: claimed=%t err=%v", claimed, err)
	}
	stale := claim
	stale.FenceToken = uuid.New()
	if started, err := repo.MarkSendStarted(context.Background(), stale); err != nil || started {
		t.Fatalf("stale fence started=%t err=%v", started, err)
	}
	if started, err := repo.MarkSendStarted(context.Background(), claim); err != nil || !started {
		t.Fatalf("start send attempt: started=%t err=%v", started, err)
	}
	if recovered, err := repo.RecoverExpiredSendAttempts(context.Background()); err != nil || len(recovered) != 0 {
		t.Fatalf("send_started must not be requeued: recovered=%v err=%v", recovered, err)
	}
	current, err := repo.GetByID(context.Background(), email.ID)
	if err != nil {
		t.Fatalf("load recovered email: %v", err)
	}
	if current.Status != domain.EmailStatusAmbiguous || current.SendAttemptState != domain.SendAttemptAmbiguous || current.AmbiguousAt == nil {
		t.Fatalf("expired send_started was not quarantined: %+v", current)
	}
	lateEventID := uuid.New()
	lateAccepted, err := NewDeliveryPipelineRepo(pool).FinalizeAccepted(context.Background(), claim, "late-provider-id",
		&domain.EmailEvent{ID: lateEventID, EmailID: email.ID, Event: "email.sent", Data: json.RawMessage(`{}`)},
		&domain.WebhookOutboxEvent{ID: uuid.New(), TeamID: teamID, EventID: lateEventID, Event: "email.sent", Payload: json.RawMessage(`{}`), RetentionClass: domain.RetentionOutbound},
	)
	if err != nil || lateAccepted {
		t.Fatalf("expired fence accepted a late provider result: accepted=%t err=%v", lateAccepted, err)
	}
	newClaim := claim
	newClaim.AttemptID = uuid.New()
	newClaim.FenceToken = uuid.New()
	if claimed, err := repo.ClaimSendAttempt(context.Background(), newClaim); err != nil || claimed {
		t.Fatalf("ambiguous attempt became sendable: claimed=%t err=%v", claimed, err)
	}

	eventID := uuid.New()
	pipeline := NewDeliveryPipelineRepo(pool)
	reconciled, err := pipeline.ReconcileAmbiguous(context.Background(), teamID, email.ID, "failed", "",
		&domain.EmailEvent{ID: eventID, EmailID: email.ID, Event: "email.failed", Data: json.RawMessage(`{"source":"test"}`)},
		&domain.WebhookOutboxEvent{ID: uuid.New(), TeamID: teamID, EventID: eventID, Event: "email.failed", Payload: json.RawMessage(`{"status":"failed"}`), RetentionClass: domain.RetentionOutbound},
	)
	if err != nil || !reconciled {
		t.Fatalf("reconcile failed outcome: reconciled=%t err=%v", reconciled, err)
	}
	current, err = repo.GetByID(context.Background(), email.ID)
	if err != nil || current.Status != domain.EmailStatusFailed || current.SendAttemptState != domain.SendAttemptFailedTerminal {
		t.Fatalf("unexpected reconciled email: email=%+v err=%v", current, err)
	}
}

func TestRecoveredLeaseRemainsDurablyPendingUntilRedisEnqueueIntegration(t *testing.T) {
	pool := integrationPool(t)
	teamID := integrationTeam(t, pool)
	email := integrationEmail(t, pool, teamID)
	repo := NewEmailRepo(pool)
	claim := domain.SendAttemptClaim{
		EmailID: email.ID, AttemptID: uuid.New(), FenceToken: uuid.New(),
		LeaseUntil: time.Now().UTC().Add(-time.Minute),
	}
	if claimed, err := repo.ClaimSendAttempt(context.Background(), claim); err != nil || !claimed {
		t.Fatalf("claim expired lease: claimed=%t err=%v", claimed, err)
	}
	if _, err := repo.RecoverExpiredSendAttempts(context.Background()); err != nil {
		t.Fatalf("recover expired lease: %v", err)
	}
	pending, err := repo.ListQueueRecoveryPending(context.Background(), 100)
	if err != nil || len(pending) != 1 || pending[0] != email.ID.String() {
		t.Fatalf("recovered email was not durably pending: pending=%v err=%v", pending, err)
	}
	if err := repo.MarkQueueRecoveryEnqueued(context.Background(), email.ID); err != nil {
		t.Fatalf("mark recovered email enqueued: %v", err)
	}
	pending, err = repo.ListQueueRecoveryPending(context.Background(), 100)
	if err != nil || len(pending) != 0 {
		t.Fatalf("completed recovery remained pending: pending=%v err=%v", pending, err)
	}
}

func TestDeadLetterReplayStartsWithFreshSendAttemptIntegration(t *testing.T) {
	pool := integrationPool(t)
	teamID := integrationTeam(t, pool)
	email := integrationEmail(t, pool, teamID)
	repo := NewEmailRepo(pool)
	claim := domain.SendAttemptClaim{
		EmailID: email.ID, AttemptID: uuid.New(), FenceToken: uuid.New(),
		LeaseUntil: time.Now().UTC().Add(time.Minute),
	}
	if claimed, err := repo.ClaimSendAttempt(context.Background(), claim); err != nil || !claimed {
		t.Fatalf("claim original attempt: claimed=%t err=%v", claimed, err)
	}
	if started, err := repo.MarkSendStarted(context.Background(), claim); err != nil || !started {
		t.Fatalf("start original attempt: started=%t err=%v", started, err)
	}
	if requeued, err := repo.MarkSendRetryable(context.Background(), claim); err != nil || !requeued {
		t.Fatalf("return retryable attempt to queue: requeued=%t err=%v", requeued, err)
	}
	if err := repo.MarkDeadLetterFailed(context.Background(), email.ID); err != nil {
		t.Fatalf("mark dead-letter terminal: %v", err)
	}
	failed, err := repo.GetByID(context.Background(), email.ID)
	if err != nil {
		t.Fatalf("load dead-lettered email: %v", err)
	}
	if failed.Status != domain.EmailStatusFailed || failed.SendAttemptState != domain.SendAttemptFailedTerminal ||
		failed.SendAttemptID != nil || failed.SendFenceToken != nil || failed.SendLeaseUntil != nil {
		t.Fatalf("dead-letter retained stale send ownership: %+v", failed)
	}
	replayToken := uuid.New()
	prepared, err := repo.PrepareDeadLetterReplay(context.Background(), teamID, email.ID, replayToken)
	if err != nil || !prepared {
		t.Fatalf("prepare dead-letter replay: prepared=%t err=%v", prepared, err)
	}
	if err := repo.MarkDeadLetterFailed(context.Background(), email.ID); err == nil {
		t.Fatal("stale dead-letter receipt closed a queued replay before its new claim")
	}
	preparedEmail, err := repo.GetByID(context.Background(), email.ID)
	if err != nil {
		t.Fatalf("load prepared replay: %v", err)
	}
	if preparedEmail.Status != domain.EmailStatusQueued || preparedEmail.SendAttemptID == nil || *preparedEmail.SendAttemptID != replayToken {
		t.Fatalf("stale dead-letter mutated prepared replay ownership: %+v", preparedEmail)
	}
	newClaim := domain.SendAttemptClaim{
		EmailID: email.ID, AttemptID: uuid.New(), FenceToken: uuid.New(),
		LeaseUntil: time.Now().UTC().Add(time.Minute),
	}
	if claimed, err := repo.ClaimSendAttempt(context.Background(), newClaim); err != nil || !claimed {
		t.Fatalf("fresh attempt could not claim replay: claimed=%t err=%v", claimed, err)
	}
	if err := repo.MarkDeadLetterFailed(context.Background(), email.ID); err == nil {
		t.Fatal("stale dead-letter receipt closed a newer active attempt")
	}
	active, err := repo.GetByID(context.Background(), email.ID)
	if err != nil {
		t.Fatalf("load active replay attempt: %v", err)
	}
	if active.Status != domain.EmailStatusSending || active.SendAttemptID == nil || *active.SendAttemptID != newClaim.AttemptID ||
		active.SendFenceToken == nil || *active.SendFenceToken != newClaim.FenceToken {
		t.Fatalf("stale dead-letter mutated new send ownership: %+v", active)
	}
}

func TestProviderInboxOutboxAndRetentionIntegration(t *testing.T) {
	pool := integrationPool(t)
	teamID := integrationTeam(t, pool)
	email := integrationEmail(t, pool, teamID)
	providerID := "provider-" + uuid.NewString()
	attemptID := uuid.New()
	if _, err := pool.Exec(context.Background(), `
		UPDATE emails SET status = 'sent', provider_message_id = $2,
			send_attempt_id = $3, send_fence_token = $4, send_attempt_state = 'accepted', sent_at = NOW()
		WHERE id = $1
	`, email.ID, providerID, attemptID, uuid.New()); err != nil {
		t.Fatalf("prepare accepted email: %v", err)
	}
	webhookID := uuid.New()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO webhooks (id, team_id, url, events, secret, active)
		VALUES ($1, $2, 'https://example.test/hook', ARRAY['email.delivered'], 'secret', true)
	`, webhookID, teamID); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	pipeline := NewDeliveryPipelineRepo(pool)
	inboxID := uuid.New()
	inbox := &domain.ProviderEventInbox{
		EventID: inboxID, ProviderMessageID: providerID, EventType: "Delivery",
		Payload: json.RawMessage(`{"eventType":"Delivery"}`), EmailID: &email.ID, SendAttemptID: &attemptID,
	}
	if err := pipeline.StoreProviderEvent(context.Background(), inbox); err != nil {
		t.Fatalf("store provider event: %v", err)
	}
	claimed, err := pipeline.ClaimProviderEvent(context.Background(), &inboxID)
	if err != nil || claimed.EventID != inboxID {
		t.Fatalf("claim provider event: event=%+v err=%v", claimed, err)
	}
	event := &domain.EmailEvent{ID: inboxID, EmailID: email.ID, Event: "email.delivered", Data: inbox.Payload}
	outbox := &domain.WebhookOutboxEvent{ID: uuid.New(), TeamID: teamID, EventID: inboxID, Event: "email.delivered", Payload: json.RawMessage(`{"status":"delivered"}`), RetentionClass: domain.RetentionOutbound}
	if err := pipeline.ApplyProviderEvent(context.Background(), inboxID, email.ID, providerID, domain.EmailStatusDelivered, event, outbox, nil); err != nil {
		t.Fatalf("apply provider event: %v", err)
	}
	if err := pipeline.ApplyProviderEvent(context.Background(), inboxID, email.ID, providerID, domain.EmailStatusDelivered, event, outbox, nil); err != nil {
		t.Fatalf("idempotent provider replay: %v", err)
	}
	if dispatched, err := pipeline.DispatchNextOutbox(context.Background()); err != nil || !dispatched {
		t.Fatalf("dispatch outbox: dispatched=%t err=%v", dispatched, err)
	}
	if dispatched, err := pipeline.DispatchNextOutbox(context.Background()); err != nil || dispatched {
		t.Fatalf("duplicate outbox dispatch: dispatched=%t err=%v", dispatched, err)
	}

	var status domain.EmailStatus
	var eventCount, deliveryCount int
	if err := pool.QueryRow(context.Background(), `SELECT status FROM emails WHERE id = $1`, email.ID).Scan(&status); err != nil || status != domain.EmailStatusDelivered {
		t.Fatalf("provider status=%s err=%v", status, err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM email_events WHERE id = $1`, inboxID).Scan(&eventCount); err != nil || eventCount != 1 {
		t.Fatalf("event count=%d err=%v", eventCount, err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM webhook_deliveries WHERE webhook_id = $1 AND event_id = $2`, webhookID, inboxID).Scan(&deliveryCount); err != nil || deliveryCount != 1 {
		t.Fatalf("delivery count=%d err=%v", deliveryCount, err)
	}

	for _, table := range []string{"provider_event_inbox", "webhook_outbox", "webhook_deliveries"} {
		if _, err := pool.Exec(context.Background(), "UPDATE "+table+" SET created_at = NOW() - INTERVAL '100 days' WHERE event_id = $1", inboxID); err != nil {
			t.Fatalf("age retained payload in %s: %v", table, err)
		}
	}
	purged, err := NewWebhookDeliveryRepo(pool).PurgeByEventClass(context.Background(), "outbound", time.Now().UTC().Add(-90*24*time.Hour))
	if err != nil || purged != 3 {
		t.Fatalf("purge retained payloads: purged=%d err=%v", purged, err)
	}
	var remaining int
	if err := pool.QueryRow(context.Background(), `
		SELECT
			(SELECT COUNT(*) FROM provider_event_inbox WHERE event_id = $1 AND payload IS NOT NULL) +
			(SELECT COUNT(*) FROM webhook_outbox WHERE event_id = $1 AND payload IS NOT NULL) +
			(SELECT COUNT(*) FROM webhook_deliveries WHERE event_id = $1 AND payload IS NOT NULL)
	`, inboxID).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("retained payload copies=%d err=%v", remaining, err)
	}
}

func TestInboundAndOutboxRollbackTogetherIntegration(t *testing.T) {
	pool := integrationPool(t)
	teamID := integrationTeam(t, pool)
	pipeline := NewDeliveryPipelineRepo(pool)
	inboundID := uuid.New()
	inbound := &domain.InboundEmail{ID: inboundID, TeamID: teamID, From: "person@example.net", To: []string{"in@example.com"}, Attachments: json.RawMessage(`[]`), Headers: json.RawMessage(`{}`)}
	outbox := &domain.WebhookOutboxEvent{ID: uuid.New(), TeamID: uuid.New(), EventID: uuid.New(), Event: "inbound.received", Payload: json.RawMessage(`{"message":"test"}`), RetentionClass: domain.RetentionInbound}
	if err := pipeline.CreateInboundWithOutbox(context.Background(), inbound, outbox); err == nil {
		t.Fatal("expected invalid outbox team to roll back transaction")
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM inbound_emails WHERE id = $1`, inboundID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("inbound row survived rollback: count=%d err=%v", count, err)
	}
}

func TestProviderInboxClaimReturnsNoRowsWhenEmptyIntegration(t *testing.T) {
	pool := integrationPool(t)
	eventID := uuid.New()
	_, err := NewDeliveryPipelineRepo(pool).ClaimProviderEvent(context.Background(), &eventID)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}
}

func TestAmbiguousAcceptanceRequiresPositiveProviderEvidenceIntegration(t *testing.T) {
	pool := integrationPool(t)
	teamID := integrationTeam(t, pool)
	email := integrationEmail(t, pool, teamID)
	attemptID := uuid.New()
	if _, err := pool.Exec(context.Background(), `
		UPDATE emails
		SET status = 'ambiguous', send_attempt_state = 'ambiguous',
			send_attempt_id = $2, ambiguous_at = NOW()
		WHERE id = $1
	`, email.ID, attemptID); err != nil {
		t.Fatalf("prepare ambiguous email: %v", err)
	}

	pipeline := NewDeliveryPipelineRepo(pool)
	providerID := "provider-" + uuid.NewString()
	negative := &domain.ProviderEventInbox{
		EventID: uuid.New(), ProviderMessageID: providerID, EventType: "Bounce",
		Payload: json.RawMessage(`{"eventType":"Bounce"}`), EmailID: &email.ID, SendAttemptID: &attemptID,
	}
	if err := pipeline.StoreProviderEvent(context.Background(), negative); err != nil {
		t.Fatalf("store negative provider evidence: %v", err)
	}
	eventID := uuid.New()
	accepted, err := pipeline.ReconcileAmbiguous(context.Background(), teamID, email.ID, "accepted", providerID,
		&domain.EmailEvent{ID: eventID, EmailID: email.ID, Event: "email.sent", Data: json.RawMessage(`{}`)},
		&domain.WebhookOutboxEvent{ID: uuid.New(), TeamID: teamID, EventID: eventID, Event: "email.sent", Payload: json.RawMessage(`{}`), RetentionClass: domain.RetentionOutbound},
	)
	if accepted || !errors.Is(err, domain.ErrDeliveryConfirmationUnavailable) {
		t.Fatalf("negative event was accepted as delivery evidence: accepted=%t err=%v", accepted, err)
	}

	positive := &domain.ProviderEventInbox{
		EventID: uuid.New(), ProviderMessageID: providerID, EventType: "Delivery",
		Payload: json.RawMessage(`{"eventType":"Delivery"}`), EmailID: &email.ID, SendAttemptID: &attemptID,
	}
	if err := pipeline.StoreProviderEvent(context.Background(), positive); err != nil {
		t.Fatalf("store positive provider evidence: %v", err)
	}
	eventID = uuid.New()
	accepted, err = pipeline.ReconcileAmbiguous(context.Background(), teamID, email.ID, "accepted", providerID,
		&domain.EmailEvent{ID: eventID, EmailID: email.ID, Event: "email.sent", Data: json.RawMessage(`{}`)},
		&domain.WebhookOutboxEvent{ID: uuid.New(), TeamID: teamID, EventID: eventID, Event: "email.sent", Payload: json.RawMessage(`{}`), RetentionClass: domain.RetentionOutbound},
	)
	if err != nil || !accepted {
		t.Fatalf("positive provider evidence did not resolve delivery: accepted=%t err=%v", accepted, err)
	}
}
