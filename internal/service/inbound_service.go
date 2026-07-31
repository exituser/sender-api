package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/pkg/webhook"
)

type InboundService struct {
	inboundRepo domain.InboundEmailRepository
	domainRepo  domain.DomainRepository
	webhookRepo domain.WebhookRepository
	logger      *slog.Logger
}

func NewInboundService(
	inboundRepo domain.InboundEmailRepository,
	domainRepo domain.DomainRepository,
	webhookRepo domain.WebhookRepository,
	logger *slog.Logger,
) *InboundService {
	return &InboundService{
		inboundRepo: inboundRepo,
		domainRepo:  domainRepo,
		webhookRepo: webhookRepo,
		logger:      logger,
	}
}

func (s *InboundService) ProcessEmail(ctx context.Context, teamID uuid.UUID, from string, to []string, subject string, text string, html string, headers map[string]string, rawS3Key string) error {
	headersJSON, _ := json.Marshal(headers)

	inbound := &domain.InboundEmail{
		ID:       uuid.New(),
		TeamID:   teamID,
		From:     from,
		To:       to,
		Subject:  &subject,
		Text:     &text,
		HTML:     &html,
		Headers:  headersJSON,
		RawS3Key: &rawS3Key,
	}

	if err := s.inboundRepo.Create(ctx, inbound); err != nil {
		return fmt.Errorf("failed to save inbound email: %w", err)
	}

	s.logger.Info("inbound email processed", "email_id", inbound.ID)

	s.dispatchWebhooks(ctx, teamID, "inbound.received", inbound)

	return nil
}

func (s *InboundService) GetByID(ctx context.Context, id uuid.UUID) (*domain.InboundEmail, error) {
	return s.inboundRepo.GetByID(ctx, id)
}

func (s *InboundService) List(ctx context.Context, teamID uuid.UUID, limit, offset int) (*domain.InboundEmailListResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	return s.inboundRepo.List(ctx, teamID, limit, offset)
}

func (s *InboundService) GetTeamByDomain(ctx context.Context, domainName string) (uuid.UUID, error) {
	domainName = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domainName), "."))
	return s.domainRepo.GetTeamByDomain(ctx, domainName)
}

func (s *InboundService) dispatchWebhooks(ctx context.Context, teamID uuid.UUID, event string, payload any) {
	webhooks, err := s.webhookRepo.GetByEvent(ctx, teamID, event)
	if err != nil {
		s.logger.Error("failed to get webhooks for inbound", "error", err)
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
			}
		}(wh)
	}
}
