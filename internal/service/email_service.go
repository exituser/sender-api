package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/pkg/validator"
	"github.com/sender-api/sender-api/pkg/webhook"
)

var ErrEmailNotDue = errors.New("email is scheduled for a later time")
var ErrEmailNotQueued = errors.New("email is no longer queued")
var ErrEmailDeliveryFailed = errors.New("email delivery failed")
var ErrQueueUnavailable = errors.New("queue unavailable")

type EmailService struct {
	emailRepo   domain.EmailRepository
	queue       domain.EmailQueue
	sender      domain.EmailSender
	webhookRepo domain.WebhookRepository
	logger      *slog.Logger
}

func NewEmailService(
	emailRepo domain.EmailRepository,
	queue domain.EmailQueue,
	sender domain.EmailSender,
	webhookRepo domain.WebhookRepository,
	logger *slog.Logger,
) *EmailService {
	return &EmailService{
		emailRepo:   emailRepo,
		queue:       queue,
		sender:      sender,
		webhookRepo: webhookRepo,
		logger:      logger,
	}
}

func (s *EmailService) Send(ctx context.Context, teamID uuid.UUID, req *domain.SendEmailRequest) (*domain.EmailResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("email request is required")
	}
	if len(req.To) == 0 {
		return nil, fmt.Errorf("at least one recipient is required")
	}
	if len(req.To) > 50 {
		return nil, fmt.Errorf("maximum 50 recipients allowed")
	}
	if req.Subject == "" {
		return nil, fmt.Errorf("subject is required")
	}
	if len(req.Subject) > 998 {
		return nil, fmt.Errorf("subject is too long")
	}
	if req.HTML == "" && req.Text == "" {
		return nil, fmt.Errorf("html or text body is required")
	}
	if req.From == "" {
		return nil, fmt.Errorf("from address is required")
	}
	if !validator.IsValidEmail(req.From) {
		return nil, fmt.Errorf("invalid from address")
	}
	for _, address := range append(append(append([]string{}, req.To...), req.CC...), req.BCC...) {
		if !validator.IsValidEmail(address) {
			return nil, fmt.Errorf("invalid recipient address: %s", address)
		}
	}
	for _, address := range req.ReplyTo {
		if !validator.IsValidEmail(address) {
			return nil, fmt.Errorf("invalid reply-to address: %s", address)
		}
	}
	for _, attachment := range req.Attachments {
		if attachment.Filename == "" || filepath.Base(attachment.Filename) != attachment.Filename {
			return nil, fmt.Errorf("invalid attachment filename")
		}
	}
	if len(req.Headers) > 50 {
		return nil, fmt.Errorf("maximum 50 custom headers allowed")
	}
	for name, value := range req.Headers {
		lowerName := strings.ToLower(strings.TrimSpace(name))
		if name == "" || len(name) > 100 || strings.TrimSpace(name) != name ||
			strings.ContainsAny(name, "\r\n:") || strings.ContainsAny(value, "\r\n") || len(value) > 2000 {
			return nil, fmt.Errorf("invalid custom header")
		}
		switch lowerName {
		case "from", "to", "cc", "bcc", "subject", "reply-to", "content-type", "mime-version":
			return nil, fmt.Errorf("reserved custom header: %s", name)
		}
	}
	totalSize := len(req.HTML) + len(req.Text)
	for _, attachment := range req.Attachments {
		totalSize += len(attachment.Content)
	}
	if totalSize > 9*1024*1024 {
		return nil, fmt.Errorf("email payload is too large")
	}

	email := &domain.Email{
		ID:          uuid.New(),
		TeamID:      teamID,
		From:        req.From,
		To:          req.To,
		CC:          req.CC,
		BCC:         req.BCC,
		Subject:     req.Subject,
		HTML:        req.HTML,
		Text:        req.Text,
		ReplyTo:     req.ReplyTo,
		Attachments: req.Attachments,
		Status:      domain.EmailStatusQueued,
		Tags:        req.Tags,
		Metadata:    req.Metadata,
		Headers:     req.Headers,
		ScheduledAt: req.ScheduledAt,
	}

	if err := s.emailRepo.Create(ctx, email); err != nil {
		return nil, fmt.Errorf("failed to save email: %w", err)
	}

	if err := s.queue.Enqueue(ctx, email.ID.String()); err != nil {
		if statusErr := s.emailRepo.UpdateStatus(ctx, email.ID, domain.EmailStatusFailed); statusErr != nil {
			s.logger.Error("failed to mark email failed after enqueue error", "email_id", email.ID, "error", statusErr)
		}
		s.recordEvent(ctx, email.ID, "email.failed", map[string]string{"reason": "queue_unavailable"})
		return nil, fmt.Errorf("%w: %v", ErrQueueUnavailable, err)
	}

	s.logger.Info("email queued", "email_id", email.ID)

	return &domain.EmailResponse{ID: email.ID.String()}, nil
}

func (s *EmailService) GetByID(ctx context.Context, teamID, id uuid.UUID) (*domain.Email, error) {
	return s.emailRepo.GetByIDForTeam(ctx, teamID, id)
}

func (s *EmailService) List(ctx context.Context, teamID uuid.UUID, limit, offset int) (*domain.EmailListResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	return s.emailRepo.List(ctx, teamID, limit, offset)
}

func (s *EmailService) GetEvents(ctx context.Context, teamID, emailID uuid.UUID) ([]domain.EmailEvent, error) {
	return s.emailRepo.GetEventsForTeam(ctx, teamID, emailID)
}

func (s *EmailService) Cancel(ctx context.Context, teamID, id uuid.UUID) error {
	email, err := s.emailRepo.GetByIDForTeam(ctx, teamID, id)
	if err != nil {
		return err
	}
	if email.Status != domain.EmailStatusQueued {
		return fmt.Errorf("only queued emails can be cancelled")
	}
	return s.emailRepo.UpdateStatusForTeam(ctx, teamID, id, domain.EmailStatusCancelled)
}

func (s *EmailService) ProcessFromQueue(ctx context.Context, emailID string) error {
	id, err := uuid.Parse(emailID)
	if err != nil {
		return fmt.Errorf("invalid email ID: %w", err)
	}

	email, err := s.emailRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("email not found: %w", err)
	}

	if email.Status != domain.EmailStatusQueued {
		return fmt.Errorf("%w: %s", ErrEmailNotQueued, email.Status)
	}
	if email.ScheduledAt != nil && time.Now().Before(*email.ScheduledAt) {
		return ErrEmailNotDue
	}

	if err := s.emailRepo.UpdateStatus(ctx, id, domain.EmailStatusSending); err != nil {
		return fmt.Errorf("failed to mark email sending: %w", err)
	}
	s.recordEvent(ctx, id, "email.sending", nil)

	if err := s.sender.Send(ctx, email); err != nil {
		if statusErr := s.emailRepo.UpdateStatus(ctx, id, domain.EmailStatusFailed); statusErr != nil {
			return fmt.Errorf("send failed: %w; failed to mark email failed: %v", err, statusErr)
		}
		s.recordEvent(ctx, id, "email.failed", map[string]string{"reason": err.Error()})
		s.dispatchWebhooks(ctx, email.TeamID, "email.failed", email)
		return fmt.Errorf("%w: %v", ErrEmailDeliveryFailed, err)
	}

	if err := s.emailRepo.UpdateStatus(ctx, id, domain.EmailStatusSent); err != nil {
		return fmt.Errorf("failed to mark email sent: %w", err)
	}
	s.recordEvent(ctx, id, "email.sent", nil)
	s.dispatchWebhooks(ctx, email.TeamID, "email.sent", email)

	return nil
}

func (s *EmailService) recordEvent(ctx context.Context, emailID uuid.UUID, eventName string, data any) {
	dataJSON := []byte("null")
	if data != nil {
		if encoded, err := json.Marshal(data); err == nil {
			dataJSON = encoded
		}
	}
	if err := s.emailRepo.AddEvent(ctx, &domain.EmailEvent{
		ID:        uuid.New(),
		EmailID:   emailID,
		Event:     eventName,
		Timestamp: time.Now().UTC(),
		Data:      dataJSON,
	}); err != nil {
		s.logger.Error("failed to record email event", "email_id", emailID, "event", eventName, "error", err)
	}
}

func (s *EmailService) dispatchWebhooks(ctx context.Context, teamID uuid.UUID, event string, payload any) {
	webhooks, err := s.webhookRepo.GetByEvent(ctx, teamID, event)
	if err != nil {
		s.logger.Error("failed to get webhooks", "error", err)
		return
	}

	for _, wh := range webhooks {
		go func(wh domain.Webhook) {
			if err := webhook.SendWebhook(wh.URL, wh.Secret, event, payload); err != nil {
				s.logger.Error("webhook delivery failed",
					"webhook_id", wh.ID,
					"event", event,
					"error", err,
				)
			} else {
				s.logger.Info("webhook delivered",
					"webhook_id", wh.ID,
					"event", event,
				)
			}
		}(wh)
	}
}

func (s *EmailService) BatchSend(ctx context.Context, teamID uuid.UUID, reqs []*domain.SendEmailRequest) ([]*domain.EmailResponse, error) {
	if len(reqs) > 100 {
		return nil, fmt.Errorf("maximum 100 emails per batch")
	}

	var responses []*domain.EmailResponse
	for _, req := range reqs {
		resp, err := s.Send(ctx, teamID, req)
		if err != nil {
			if req == nil {
				return responses, fmt.Errorf("failed to send email: %w", err)
			}
			return responses, fmt.Errorf("failed to send email to %s: %w", strings.Join(req.To, ","), err)
		}
		responses = append(responses, resp)
	}
	return responses, nil
}
