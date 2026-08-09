package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/pkg/metrics"
)

type EmailRetentionRepository interface {
	PurgeEmailsBefore(context.Context, time.Time) (int64, error)
}

type InboundRetentionRepository interface {
	ListExpired(context.Context, time.Time, int) ([]domain.ExpiredInboundRecord, error)
	DeleteExpired(context.Context, uuid.UUID, time.Time) (bool, error)
}

type InboundObjectDeleter interface {
	DeleteInboundObject(context.Context, string) error
}

type InboundObjectDeleteFunc func(context.Context, string) error

func (f InboundObjectDeleteFunc) DeleteInboundObject(ctx context.Context, key string) error {
	return f(ctx, key)
}

type WebhookPayloadRetentionRepository interface {
	PurgeByEventClass(context.Context, string, time.Time) (int64, error)
}

type RetentionService struct {
	emails   EmailRetentionRepository
	inbound  InboundRetentionRepository
	webhooks WebhookPayloadRetentionRepository
	objects  InboundObjectDeleter
}

func (s *RetentionService) SetInboundObjectDeleter(deleter InboundObjectDeleter) {
	s.objects = deleter
}

func NewRetentionService(emails EmailRetentionRepository, inbound InboundRetentionRepository, webhooks ...WebhookPayloadRetentionRepository) *RetentionService {
	var webhookRepo WebhookPayloadRetentionRepository
	if len(webhooks) > 0 {
		webhookRepo = webhooks[0]
	}
	return &RetentionService{emails: emails, inbound: inbound, webhooks: webhookRepo}
}

func (s *RetentionService) Purge(ctx context.Context, emailAge, inboundAge time.Duration) error {
	if emailAge <= 0 && inboundAge <= 0 {
		return fmt.Errorf("retention is disabled")
	}
	now := time.Now().UTC()
	emailBefore := now.Add(-emailAge)
	inboundBefore := now.Add(-inboundAge)

	// Remove duplicated webhook/provider payloads before deleting their source
	// rows. If this step fails, the next retention pass can retry without
	// leaving an unnoticed payload copy behind.
	if s.webhooks != nil {
		if emailAge > 0 {
			count, err := s.webhooks.PurgeByEventClass(ctx, "outbound", emailBefore)
			if err != nil {
				return fmt.Errorf("purge outbound webhook payloads: %w", err)
			}
			metrics.AddCounter("sender_api_retention_outbound_payloads_deleted_total", uint64(count))
		}
		if inboundAge > 0 {
			count, err := s.webhooks.PurgeByEventClass(ctx, "inbound", inboundBefore)
			if err != nil {
				return fmt.Errorf("purge inbound webhook payloads: %w", err)
			}
			metrics.AddCounter("sender_api_retention_inbound_payloads_deleted_total", uint64(count))
		}
	}
	if emailAge > 0 && s.emails != nil {
		count, err := s.emails.PurgeEmailsBefore(ctx, emailBefore)
		if err != nil {
			return err
		}
		metrics.AddCounter("sender_api_retention_emails_deleted_total", uint64(count))
	}
	if inboundAge > 0 && s.inbound != nil {
		count, err := s.purgeInbound(ctx, inboundBefore)
		if err != nil {
			return fmt.Errorf("purge inbound messages: %w", err)
		}
		metrics.AddCounter("sender_api_retention_inbound_deleted_total", uint64(count))
	}
	return nil
}

func (s *RetentionService) purgeInbound(ctx context.Context, before time.Time) (int64, error) {
	const batchSize = 500
	var deleted int64
	for {
		items, err := s.inbound.ListExpired(ctx, before, batchSize)
		if err != nil {
			return deleted, err
		}
		if len(items) == 0 {
			return deleted, nil
		}
		for _, item := range items {
			key := strings.TrimSpace(item.RawObjectKey)
			if key != "" {
				if s.objects == nil {
					return deleted, fmt.Errorf("raw message cleanup is not configured")
				}
				if err := s.objects.DeleteInboundObject(ctx, key); err != nil {
					return deleted, fmt.Errorf("delete raw message object: %w", err)
				}
				metrics.AddCounter("sender_api_retention_inbound_objects_deleted_total", 1)
			}
			removed, err := s.inbound.DeleteExpired(ctx, item.ID, before)
			if err != nil {
				return deleted, err
			}
			if removed {
				deleted++
			}
		}
		if len(items) < batchSize {
			return deleted, nil
		}
	}
}
