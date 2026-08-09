package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/pkg/metrics"
)

var ErrProviderEmailNotFound = errors.New("email for provider event was not found")

func (s *EmailService) ProcessProviderEvent(ctx context.Context, providerMessageID, eventType string, data json.RawMessage, eventID uuid.UUID) error {
	if s.pipelineRepo != nil {
		return s.storeAndProcessProviderEvent(ctx, providerMessageID, eventType, data, eventID)
	}
	return s.processProviderEventLegacy(ctx, providerMessageID, eventType, data, eventID)
}

func (s *EmailService) storeAndProcessProviderEvent(ctx context.Context, providerMessageID, eventType string, data json.RawMessage, eventID uuid.UUID) error {
	providerMessageID = strings.TrimSpace(providerMessageID)
	if providerMessageID == "" {
		return fmt.Errorf("provider message id is required")
	}
	eventName, _, _ := providerEventDetails(eventType)
	if eventName == "" {
		return nil
	}
	if eventID == uuid.Nil {
		eventID = uuid.NewSHA1(uuid.Nil, []byte(providerMessageID+":"+eventName))
	}
	emailID, attemptID := providerEventCorrelation(data)
	inbox := &domain.ProviderEventInbox{
		EventID: eventID, ProviderMessageID: providerMessageID,
		EventType: eventType, Payload: data, EmailID: emailID, SendAttemptID: attemptID,
	}
	if err := s.pipelineRepo.StoreProviderEvent(ctx, inbox); err != nil {
		return fmt.Errorf("store provider event inbox: %w", err)
	}
	metrics.AddCounter("sender_api_provider_events_received_total", 1)
	if _, err := s.processProviderEventInbox(ctx, &eventID); err != nil {
		// The authenticated callback is already durable. A worker will retry it;
		// returning 2xx avoids multiplying the same durable inbox item at SNS.
		s.logger.Error("provider event stored for retry", "event_id", eventID, "error", err)
	}
	return nil
}

func (s *EmailService) ProcessNextProviderEvent(ctx context.Context) (bool, error) {
	if s.pipelineRepo == nil {
		return false, nil
	}
	return s.processProviderEventInbox(ctx, nil)
}

func (s *EmailService) processProviderEventInbox(ctx context.Context, eventID *uuid.UUID) (bool, error) {
	inbox, err := s.pipelineRepo.ClaimProviderEvent(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim provider event inbox: %w", err)
	}

	eventName, targetStatus, _ := providerEventDetails(inbox.EventType)
	if eventName == "" {
		if err := s.pipelineRepo.RetryProviderEvent(ctx, inbox.EventID, "unsupported provider event", time.Now().UTC(), true); err != nil {
			return true, err
		}
		metrics.AddCounter("sender_api_provider_events_ignored_total", 1)
		return true, nil
	}

	email, lookupErr := s.emailRepo.GetByProviderMessageID(ctx, inbox.ProviderMessageID)
	if lookupErr != nil && !errors.Is(lookupErr, pgx.ErrNoRows) {
		s.retryProviderInbox(ctx, inbox, lookupErr)
		return true, fmt.Errorf("lookup provider email: %w", lookupErr)
	}
	if email == nil && inbox.EmailID != nil && inbox.SendAttemptID != nil {
		candidate, candidateErr := s.emailRepo.GetByID(ctx, *inbox.EmailID)
		if candidateErr != nil && !errors.Is(candidateErr, pgx.ErrNoRows) {
			s.retryProviderInbox(ctx, inbox, candidateErr)
			return true, fmt.Errorf("lookup tagged provider email: %w", candidateErr)
		}
		if candidate != nil && (candidate.SendAttemptID == nil || *candidate.SendAttemptID != *inbox.SendAttemptID) {
			if err := s.pipelineRepo.RetryProviderEvent(ctx, inbox.EventID, "provider attempt tag does not match", time.Now().UTC(), true); err != nil {
				return true, err
			}
			return true, nil
		}
		email = candidate
	}
	if email == nil {
		terminal := inbox.Attempts >= 20 || (!inbox.CreatedAt.IsZero() && time.Since(inbox.CreatedAt) > 7*24*time.Hour)
		reason := "provider email correlation is pending"
		if terminal {
			reason = "provider email could not be correlated before retry limit"
		}
		if err := s.pipelineRepo.RetryProviderEvent(ctx, inbox.EventID, reason, providerRetryAt(inbox.Attempts), terminal); err != nil {
			return true, err
		}
		if terminal {
			metrics.AddCounter("sender_api_provider_events_ignored_total", 1)
		} else {
			metrics.AddCounter("sender_api_provider_events_retried_total", 1)
		}
		return true, nil
	}

	if domain.ShouldApplyProviderStatus(email.Status, targetStatus) {
		email.Status = targetStatus
		if targetStatus == domain.EmailStatusFailed {
			email.SendAttemptState = domain.SendAttemptFailedTerminal
		} else if targetStatus != "" {
			email.SendAttemptState = domain.SendAttemptAccepted
		}
	}
	providerID := inbox.ProviderMessageID
	email.ProviderMessageID = &providerID
	payload, err := json.Marshal(email)
	if err != nil {
		s.retryProviderInbox(ctx, inbox, err)
		return true, fmt.Errorf("marshal provider webhook payload: %w", err)
	}
	event := &domain.EmailEvent{
		ID: inbox.EventID, EmailID: email.ID, Event: eventName,
		Timestamp: time.Now().UTC(), Data: inbox.Payload,
	}
	outbox := &domain.WebhookOutboxEvent{
		ID:     uuid.NewSHA1(inbox.EventID, []byte("webhook-outbox")),
		TeamID: email.TeamID, EventID: inbox.EventID, Event: eventName,
		Payload: payload, RetentionClass: domain.RetentionOutbound,
	}
	suppressions := providerEventSuppressions(email.TeamID, targetStatus, inbox.Payload)
	if err := s.pipelineRepo.ApplyProviderEvent(ctx, inbox.EventID, email.ID, inbox.ProviderMessageID, targetStatus, event, outbox, suppressions); err != nil {
		s.retryProviderInbox(ctx, inbox, err)
		return true, fmt.Errorf("apply provider event: %w", err)
	}
	return true, nil
}

func (s *EmailService) retryProviderInbox(ctx context.Context, inbox *domain.ProviderEventInbox, cause error) {
	terminal := providerRetryIsTerminal(inbox, cause, time.Now().UTC())
	if err := s.pipelineRepo.RetryProviderEvent(ctx, inbox.EventID, cause.Error(), providerRetryAt(inbox.Attempts), terminal); err != nil {
		s.logger.Error("failed to schedule provider event retry", "event_id", inbox.EventID, "error", err)
		return
	}
	if terminal {
		metrics.AddCounter("sender_api_provider_events_ignored_total", 1)
	} else {
		metrics.AddCounter("sender_api_provider_events_retried_total", 1)
	}
}

func providerRetryIsTerminal(inbox *domain.ProviderEventInbox, cause error, now time.Time) bool {
	if errors.Is(cause, domain.ErrProviderCorrelationMismatch) {
		return true
	}
	return inbox != nil && !inbox.CreatedAt.IsZero() && now.Sub(inbox.CreatedAt) >= 7*24*time.Hour
}

func providerRetryAt(attempts int) time.Time {
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 8 {
		attempts = 8
	}
	return time.Now().UTC().Add(time.Duration(1<<uint(attempts-1)) * time.Second)
}

func providerEventCorrelation(data json.RawMessage) (*uuid.UUID, *uuid.UUID) {
	var payload struct {
		Mail struct {
			Tags map[string][]string `json:"tags"`
		} `json:"mail"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return nil, nil
	}
	parseTag := func(name string) *uuid.UUID {
		values := payload.Mail.Tags[name]
		if len(values) != 1 {
			return nil
		}
		value, err := uuid.Parse(strings.TrimSpace(values[0]))
		if err != nil || value == uuid.Nil {
			return nil
		}
		return &value
	}
	return parseTag("sender_email_id"), parseTag("sender_attempt_id")
}

func providerEventSuppressions(teamID uuid.UUID, status domain.EmailStatus, data json.RawMessage) []domain.Suppression {
	if status != domain.EmailStatusBounced && status != domain.EmailStatusComplained {
		return nil
	}
	var event struct {
		Bounce struct {
			BouncedRecipients []struct {
				EmailAddress string `json:"emailAddress"`
			} `json:"bouncedRecipients"`
		} `json:"bounce"`
		Complaint struct {
			ComplainedRecipients []struct {
				EmailAddress string `json:"emailAddress"`
			} `json:"complainedRecipients"`
		} `json:"complaint"`
	}
	if json.Unmarshal(data, &event) != nil {
		return nil
	}
	reason := domain.SuppressionReasonBounce
	recipients := event.Bounce.BouncedRecipients
	if status == domain.EmailStatusComplained {
		reason = domain.SuppressionReasonComplaint
		recipients = event.Complaint.ComplainedRecipients
	}
	result := make([]domain.Suppression, 0, len(recipients))
	seen := make(map[string]struct{})
	for _, recipient := range recipients {
		email := domain.NormalizeEmail(recipient.EmailAddress)
		if email == "" {
			continue
		}
		if _, exists := seen[email]; exists {
			continue
		}
		seen[email] = struct{}{}
		result = append(result, domain.Suppression{ID: uuid.New(), TeamID: teamID, Email: email, Reason: reason})
	}
	return result
}

func (s *EmailService) processProviderEventLegacy(ctx context.Context, providerMessageID, eventType string, data json.RawMessage, eventID uuid.UUID) error {
	providerMessageID = strings.TrimSpace(providerMessageID)
	if providerMessageID == "" {
		return fmt.Errorf("provider message id is required")
	}

	eventName, targetStatus, hasStatus := providerEventDetails(eventType)
	if eventName == "" {
		return nil
	}

	email, err := s.emailRepo.GetByProviderMessageID(ctx, providerMessageID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrProviderEmailNotFound, err)
	}
	if eventID == uuid.Nil {
		eventID = uuid.NewSHA1(uuid.Nil, []byte(providerMessageID+":"+eventName))
	}

	statusChanged := hasStatus && shouldApplyProviderStatus(email.Status, targetStatus)
	if statusChanged {
		if err := s.emailRepo.UpdateStatus(ctx, email.ID, targetStatus); err != nil {
			return fmt.Errorf("update email provider status: %w", err)
		}
		email.Status = targetStatus
	}
	if targetStatus == domain.EmailStatusBounced || targetStatus == domain.EmailStatusComplained {
		if err := s.suppressEventRecipients(ctx, email.TeamID, targetStatus, data); err != nil {
			return err
		}
	}

	if statusChanged || !hasStatus {
		dataJSON := data
		if err := s.recordProviderEventAndDispatch(ctx, email.TeamID, email.ID, eventID, eventName, dataJSON, email); err != nil {
			return err
		}
	} else if err := s.emailRepo.AddEvent(ctx, &domain.EmailEvent{
		ID: eventID, EmailID: email.ID, Event: eventName, Timestamp: time.Now().UTC(), Data: data,
	}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil
		}
		return fmt.Errorf("record provider event: %w", err)
	}
	return nil
}

func (s *EmailService) recordProviderEventAndDispatch(ctx context.Context, teamID, emailID, eventID uuid.UUID, eventName string, data json.RawMessage, payload any) error {
	if s.webhookRepo == nil {
		if err := s.emailRepo.AddEvent(ctx, &domain.EmailEvent{ID: eventID, EmailID: emailID, Event: eventName, Timestamp: time.Now().UTC(), Data: data}); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return nil
			}
			return fmt.Errorf("record provider event: %w", err)
		}
		return nil
	}

	webhooks, err := s.webhookRepo.GetByEvent(ctx, teamID, eventName)
	if err != nil {
		return fmt.Errorf("get provider event webhooks: %w", err)
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal provider event webhook: %w", err)
	}
	deliveries := make([]*domain.WebhookDelivery, 0, len(webhooks))
	for _, webhook := range webhooks {
		deliveries = append(deliveries, &domain.WebhookDelivery{ID: uuid.New(), WebhookID: webhook.ID, EventID: eventID, Event: eventName, Payload: payloadJSON})
	}
	if err := s.emailRepo.AddEventWithDeliveries(ctx, &domain.EmailEvent{ID: eventID, EmailID: emailID, Event: eventName, Timestamp: time.Now().UTC(), Data: data}, deliveries); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil
		}
		return fmt.Errorf("record provider event and webhook deliveries: %w", err)
	}
	return nil
}

func (s *EmailService) suppressEventRecipients(ctx context.Context, teamID uuid.UUID, status domain.EmailStatus, data json.RawMessage) error {
	if s.suppressionRepo == nil {
		return nil
	}

	var event struct {
		Bounce struct {
			BouncedRecipients []struct {
				EmailAddress string `json:"emailAddress"`
			} `json:"bouncedRecipients"`
		} `json:"bounce"`
		Complaint struct {
			ComplainedRecipients []struct {
				EmailAddress string `json:"emailAddress"`
			} `json:"complainedRecipients"`
		} `json:"complaint"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("decode provider suppression recipients: %w", err)
	}

	var recipients []string
	reason := domain.SuppressionReasonBounce
	if status == domain.EmailStatusBounced {
		for _, recipient := range event.Bounce.BouncedRecipients {
			recipients = append(recipients, recipient.EmailAddress)
		}
	} else {
		reason = domain.SuppressionReasonComplaint
		for _, recipient := range event.Complaint.ComplainedRecipients {
			recipients = append(recipients, recipient.EmailAddress)
		}
	}
	for _, recipient := range recipients {
		email := domain.NormalizeEmail(recipient)
		if email == "" {
			continue
		}
		if err := s.suppressionRepo.Upsert(ctx, &domain.Suppression{
			ID: uuid.New(), TeamID: teamID, Email: email, Reason: reason,
		}); err != nil {
			return fmt.Errorf("upsert recipient suppression: %w", err)
		}
	}
	return nil
}

func providerEventDetails(eventType string) (string, domain.EmailStatus, bool) {
	canonical := strings.ToLower(strings.TrimSpace(eventType))
	canonical = strings.ReplaceAll(canonical, " ", "_")
	if canonical == "" {
		return "", "", false
	}

	switch canonical {
	case "send":
		return "email.sent", domain.EmailStatusSent, true
	case "delivery":
		return "email.delivered", domain.EmailStatusDelivered, true
	case "bounce":
		return "email.bounced", domain.EmailStatusBounced, true
	case "complaint":
		return "email.complained", domain.EmailStatusComplained, true
	case "reject", "rendering_failure":
		return "email.failed", domain.EmailStatusFailed, true
	case "open":
		return "email.opened", domain.EmailStatusOpened, true
	case "click":
		return "email.clicked", domain.EmailStatusClicked, true
	case "deliverydelay", "delivery_delay":
		return "email.delivery_delay", "", false
	case "subscription":
		return "email.subscription", "", false
	default:
		return "", "", false
	}
}

func shouldApplyProviderStatus(current, target domain.EmailStatus) bool {
	return domain.ShouldApplyProviderStatus(current, target)
}

func providerStatusRank(status domain.EmailStatus) int {
	return domain.ProviderStatusRank(status)
}
