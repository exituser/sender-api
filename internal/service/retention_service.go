package service

import (
	"context"
	"fmt"
	"time"
)

type EmailRetentionRepository interface {
	PurgeEmailsBefore(context.Context, time.Time) (int64, error)
}

type InboundRetentionRepository interface {
	PurgeBefore(context.Context, time.Time) (int64, error)
}

type RetentionService struct {
	emails  EmailRetentionRepository
	inbound InboundRetentionRepository
}

func NewRetentionService(emails EmailRetentionRepository, inbound InboundRetentionRepository) *RetentionService {
	return &RetentionService{emails: emails, inbound: inbound}
}

func (s *RetentionService) Purge(ctx context.Context, emailAge, inboundAge time.Duration) error {
	if emailAge > 0 && s.emails != nil {
		if _, err := s.emails.PurgeEmailsBefore(ctx, time.Now().UTC().Add(-emailAge)); err != nil {
			return err
		}
	}
	if inboundAge > 0 && s.inbound != nil {
		if _, err := s.inbound.PurgeBefore(ctx, time.Now().UTC().Add(-inboundAge)); err != nil {
			return err
		}
	}
	if emailAge <= 0 && inboundAge <= 0 {
		return fmt.Errorf("retention is disabled")
	}
	return nil
}
