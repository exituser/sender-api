package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sender-api/sender-api/internal/domain"
)

var ErrProviderEmailNotFound = errors.New("email for provider event was not found")

func (s *EmailService) ProcessProviderEvent(ctx context.Context, providerMessageID, eventType string, data json.RawMessage, eventID uuid.UUID) error {
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

	if err := s.emailRepo.AddEvent(ctx, &domain.EmailEvent{
		ID:        eventID,
		EmailID:   email.ID,
		Event:     eventName,
		Timestamp: time.Now().UTC(),
		Data:      data,
	}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil
		}
		return fmt.Errorf("record provider event: %w", err)
	}

	if statusChanged || !hasStatus {
		s.dispatchWebhooks(ctx, email.TeamID, eventID, eventName, email)
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
	if target == "" || current == domain.EmailStatusCancelled || current == domain.EmailStatusFailed {
		return false
	}
	if target == domain.EmailStatusFailed {
		return current != domain.EmailStatusBounced && current != domain.EmailStatusComplained
	}
	if target == domain.EmailStatusBounced || target == domain.EmailStatusComplained {
		return true
	}
	if current == domain.EmailStatusBounced || current == domain.EmailStatusComplained {
		return false
	}
	return providerStatusRank(target) > providerStatusRank(current)
}

func providerStatusRank(status domain.EmailStatus) int {
	switch status {
	case domain.EmailStatusQueued:
		return 0
	case domain.EmailStatusSending:
		return 1
	case domain.EmailStatusSent:
		return 2
	case domain.EmailStatusDelivered:
		return 3
	case domain.EmailStatusOpened:
		return 4
	case domain.EmailStatusClicked:
		return 5
	default:
		return -1
	}
}
