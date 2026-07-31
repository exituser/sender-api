package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/pkg/validator"
)

type InboundService struct {
	inboundRepo  domain.InboundEmailRepository
	domainRepo   domain.DomainRepository
	webhookRepo  domain.WebhookRepository
	deliveryRepo domain.WebhookDeliveryRepository
	logger       *slog.Logger
}

func NewInboundService(
	inboundRepo domain.InboundEmailRepository,
	domainRepo domain.DomainRepository,
	webhookRepo domain.WebhookRepository,
	deliveryRepo domain.WebhookDeliveryRepository,
	logger *slog.Logger,
) *InboundService {
	return &InboundService{
		inboundRepo:  inboundRepo,
		domainRepo:   domainRepo,
		webhookRepo:  webhookRepo,
		deliveryRepo: deliveryRepo,
		logger:       logger,
	}
}

func (s *InboundService) ProcessEmail(ctx context.Context, teamID uuid.UUID, from string, to []string, subject string, text string, html string, headers map[string]string, rawS3Key string) error {
	return s.ProcessEmailWithMessageID(ctx, teamID, nil, from, to, subject, text, html, headers, rawS3Key)
}

func (s *InboundService) ProcessEmailWithMessageID(ctx context.Context, teamID uuid.UUID, messageID *string, from string, to []string, subject string, text string, html string, headers map[string]string, rawS3Key string) error {
	if messageID != nil && strings.TrimSpace(*messageID) != "" {
		if _, err := s.inboundRepo.GetByMessageID(ctx, teamID, strings.TrimSpace(*messageID)); err == nil {
			return nil
		}
	}

	headersJSON, _ := json.Marshal(headers)

	inbound := &domain.InboundEmail{
		ID:        uuid.New(),
		TeamID:    teamID,
		MessageID: messageID,
		From:      from,
		To:        to,
		Subject:   &subject,
		Text:      &text,
		HTML:      &html,
		Headers:   headersJSON,
		RawS3Key:  &rawS3Key,
	}

	if err := s.inboundRepo.Create(ctx, inbound); err != nil {
		var pgErr *pgconn.PgError
		if messageID != nil && errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil
		}
		return fmt.Errorf("failed to save inbound email: %w", err)
	}

	s.logger.Info("inbound email processed", "email_id", inbound.ID)

	s.dispatchWebhooks(ctx, teamID, uuid.New(), "inbound.received", inbound)

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
	if offset < 0 {
		offset = 0
	}
	return s.inboundRepo.List(ctx, teamID, limit, offset)
}

func (s *InboundService) GetTeamByDomain(ctx context.Context, domainName string) (uuid.UUID, error) {
	domainName = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domainName), "."))
	return s.domainRepo.GetTeamByDomain(ctx, domainName)
}

func (s *InboundService) TeamForRecipients(ctx context.Context, recipients []string) (uuid.UUID, error) {
	var teamID uuid.UUID
	if len(recipients) == 0 || len(recipients) > 50 {
		return uuid.Nil, fmt.Errorf("invalid recipient count")
	}
	for _, recipient := range recipients {
		domainPart := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(validator.EmailDomain(recipient)), "."))
		if domainPart == "" {
			return uuid.Nil, fmt.Errorf("invalid recipient domain")
		}
		candidate, err := s.GetTeamByDomain(ctx, domainPart)
		if err != nil {
			return uuid.Nil, err
		}
		if teamID == uuid.Nil {
			teamID = candidate
			continue
		}
		if candidate != teamID {
			return uuid.Nil, fmt.Errorf("recipients belong to different teams")
		}
	}
	return teamID, nil
}

func (s *InboundService) dispatchWebhooks(ctx context.Context, teamID, eventID uuid.UUID, event string, payload any) {
	if s.deliveryRepo == nil {
		s.logger.Error("webhook delivery repository is not configured", "event", event)
		return
	}
	webhooks, err := s.webhookRepo.GetByEvent(ctx, teamID, event)
	if err != nil {
		s.logger.Error("failed to get webhooks for inbound", "error", err)
		return
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		s.logger.Error("failed to marshal inbound webhook payload", "event", event, "error", err)
		return
	}
	for _, wh := range webhooks {
		if err := s.deliveryRepo.CreateDelivery(ctx, &domain.WebhookDelivery{
			ID:        uuid.New(),
			WebhookID: wh.ID,
			EventID:   eventID,
			Event:     event,
			Payload:   payloadJSON,
		}); err != nil {
			s.logger.Error("failed to enqueue inbound webhook delivery", "webhook_id", wh.ID, "event", event, "error", err)
		}
	}
}
