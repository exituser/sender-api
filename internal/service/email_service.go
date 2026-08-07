package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/pkg/validator"
)

var ErrEmailNotDue = errors.New("email is scheduled for a later time")

type EmailNotDueError struct {
	At time.Time
}

func (e *EmailNotDueError) Error() string {
	return ErrEmailNotDue.Error()
}

func (e *EmailNotDueError) Unwrap() error {
	return ErrEmailNotDue
}

var ErrEmailNotQueued = errors.New("email is no longer queued")
var ErrEmailDeliveryFailed = errors.New("email delivery failed")
var ErrEmailDeliveryRetryable = errors.New("email delivery should be retried")
var ErrQueueUnavailable = errors.New("queue unavailable")
var ErrIdempotencyConflict = errors.New("idempotency key was already used with a different request")
var ErrBatchIdempotencyKeyRequired = errors.New("Idempotency-Key is required for batch sends")
var ErrDailyRecipientLimit = errors.New("daily recipient limit exceeded")
var ErrUsageUnavailable = errors.New("usage limiter unavailable")
var ErrRecipientSuppressed = errors.New("recipient is suppressed")
var ErrEmailAccepted = errors.New("email was accepted by the provider but could not be fully persisted")

type providerAcceptedEmailRepository interface {
	MarkProviderAccepted(ctx context.Context, id uuid.UUID, messageID string) error
}

type EmailService struct {
	emailRepo       domain.EmailRepository
	domainRepo      domain.DomainRepository
	queue           domain.EmailQueue
	sender          domain.EmailSender
	webhookRepo     domain.WebhookRepository
	deliveryRepo    domain.WebhookDeliveryRepository
	suppressionRepo domain.SuppressionRepository
	teamRepo        domain.TeamRepository
	logger          *slog.Logger
	usageLimiter    domain.UsageLimiter
	dailyLimit      int
	planLimits      map[domain.Plan]int
}

func (s *EmailService) SetUsageLimiter(limiter domain.UsageLimiter, dailyLimit int) {
	s.usageLimiter = limiter
	s.dailyLimit = dailyLimit
}

func NewEmailService(
	emailRepo domain.EmailRepository,
	domainRepo domain.DomainRepository,
	queue domain.EmailQueue,
	sender domain.EmailSender,
	webhookRepo domain.WebhookRepository,
	deliveryRepo domain.WebhookDeliveryRepository,
	logger *slog.Logger,
	suppressionRepos ...domain.SuppressionRepository,
) *EmailService {
	var suppressionRepo domain.SuppressionRepository
	if len(suppressionRepos) > 0 {
		suppressionRepo = suppressionRepos[0]
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &EmailService{
		emailRepo:       emailRepo,
		domainRepo:      domainRepo,
		queue:           queue,
		sender:          sender,
		webhookRepo:     webhookRepo,
		deliveryRepo:    deliveryRepo,
		suppressionRepo: suppressionRepo,
		logger:          logger,
	}
}

func (s *EmailService) SetPlanResolver(teamRepo domain.TeamRepository, freeLimit, proLimit, scaleLimit int) {
	s.teamRepo = teamRepo
	s.planLimits = map[domain.Plan]int{
		domain.PlanFree:  freeLimit,
		domain.PlanPro:   proLimit,
		domain.PlanScale: scaleLimit,
	}
}

// SetSuppressionRepository enables bounce and complaint suppression for
// existing composition roots without requiring constructor-call changes.
func (s *EmailService) SetSuppressionRepository(repo domain.SuppressionRepository) {
	s.suppressionRepo = repo
}

func (s *EmailService) Send(ctx context.Context, teamID uuid.UUID, req *domain.SendEmailRequest) (*domain.EmailResponse, error) {
	response, _, err := s.send(ctx, teamID, req, "")
	return response, err
}

func (s *EmailService) SendWithIdempotency(ctx context.Context, teamID uuid.UUID, req *domain.SendEmailRequest, idempotencyKey string) (*domain.EmailResponse, bool, error) {
	return s.send(ctx, teamID, req, strings.TrimSpace(idempotencyKey))
}

func (s *EmailService) send(ctx context.Context, teamID uuid.UUID, req *domain.SendEmailRequest, idempotencyKey string) (*domain.EmailResponse, bool, error) {
	if req == nil {
		return nil, false, fmt.Errorf("email request is required")
	}
	req.From = domain.NormalizeEmail(req.From)
	if len(req.To) == 0 {
		return nil, false, fmt.Errorf("at least one recipient is required")
	}
	if len(req.To)+len(req.CC)+len(req.BCC) > 50 {
		return nil, false, fmt.Errorf("maximum 50 recipients allowed")
	}
	if req.Subject == "" {
		return nil, false, fmt.Errorf("subject is required")
	}
	if len(req.Subject) > 998 {
		return nil, false, fmt.Errorf("subject is too long")
	}
	if req.HTML == "" && req.Text == "" {
		return nil, false, fmt.Errorf("html or text body is required")
	}
	if req.From == "" {
		return nil, false, fmt.Errorf("from address is required")
	}
	if !validator.IsValidEmail(req.From) {
		return nil, false, fmt.Errorf("invalid from address")
	}
	fromDomain := validator.EmailDomain(req.From)
	if s.domainRepo == nil {
		return nil, false, fmt.Errorf("sender domain authorization is not configured")
	}
	configuredDomain, err := s.domainRepo.GetByName(ctx, teamID, fromDomain)
	if err != nil || configuredDomain == nil || configuredDomain.Status != domain.DomainStatusVerified {
		return nil, false, fmt.Errorf("from domain is not verified for this team")
	}
	if idempotencyKey != "" {
		if len(idempotencyKey) > 255 || strings.ContainsAny(idempotencyKey, "\r\n") {
			return nil, false, fmt.Errorf("invalid idempotency key")
		}
		requestHash := hashSendRequest(req)
		existing, lookupErr := s.emailRepo.GetByIdempotencyKey(ctx, teamID, idempotencyKey)
		if lookupErr == nil && existing != nil {
			if existing.IdempotencyHash == nil || *existing.IdempotencyHash != requestHash {
				return nil, false, ErrIdempotencyConflict
			}
			return &domain.EmailResponse{ID: existing.ID.String(), Idempotent: true}, false, nil
		}
		if lookupErr != nil && !errors.Is(lookupErr, pgx.ErrNoRows) {
			return nil, false, fmt.Errorf("failed to check idempotency key: %w", lookupErr)
		}
	}
	recipients := normalizeRecipients(req)
	for _, address := range recipients {
		if !validator.IsValidEmail(address) {
			return nil, false, fmt.Errorf("invalid recipient address: %s", address)
		}
	}
	if s.suppressionRepo != nil {
		suppressions, err := s.suppressionRepo.GetByEmails(ctx, teamID, recipients)
		if err != nil {
			return nil, false, fmt.Errorf("check recipient suppressions: %w", err)
		}
		if len(suppressions) > 0 {
			return nil, false, fmt.Errorf("%w: %s (%s)", ErrRecipientSuppressed, suppressions[0].Email, suppressions[0].Reason)
		}
	}
	for _, address := range req.ReplyTo {
		if !validator.IsValidEmail(address) {
			return nil, false, fmt.Errorf("invalid reply-to address: %s", address)
		}
	}
	for _, attachment := range req.Attachments {
		if attachment.Filename == "" || filepath.Base(attachment.Filename) != attachment.Filename {
			return nil, false, fmt.Errorf("invalid attachment filename")
		}
	}
	if len(req.Headers) > 50 {
		return nil, false, fmt.Errorf("maximum 50 custom headers allowed")
	}
	if len(req.Attachments) > 10 {
		return nil, false, fmt.Errorf("maximum 10 attachments allowed")
	}
	if len(req.Tags) > 50 {
		return nil, false, fmt.Errorf("maximum 50 tags allowed")
	}
	if len(req.Metadata) > 50 {
		return nil, false, fmt.Errorf("maximum 50 metadata fields allowed")
	}
	for _, tag := range req.Tags {
		if strings.TrimSpace(tag.Name) == "" || len(tag.Name) > 100 || len(tag.Value) > 500 ||
			strings.ContainsAny(tag.Name, "\r\n") || strings.ContainsAny(tag.Value, "\r\n") {
			return nil, false, fmt.Errorf("invalid email tag")
		}
	}
	for key, value := range req.Metadata {
		if strings.TrimSpace(key) == "" || len(key) > 100 || len(value) > 1000 ||
			strings.ContainsAny(key, "\r\n") || strings.ContainsAny(value, "\r\n") {
			return nil, false, fmt.Errorf("invalid email metadata")
		}
	}
	for name, value := range req.Headers {
		lowerName := strings.ToLower(strings.TrimSpace(name))
		if name == "" || len(name) > 100 || strings.TrimSpace(name) != name ||
			strings.ContainsAny(name, "\r\n:") || strings.ContainsAny(value, "\r\n") || len(value) > 2000 {
			return nil, false, fmt.Errorf("invalid custom header")
		}
		switch lowerName {
		case "from", "to", "cc", "bcc", "subject", "reply-to", "content-type", "mime-version":
			return nil, false, fmt.Errorf("reserved custom header: %s", name)
		}
	}
	totalSize := len(req.HTML) + len(req.Text)
	for _, attachment := range req.Attachments {
		totalSize += len(attachment.Content)
	}
	if totalSize > 7*1024*1024 {
		return nil, false, fmt.Errorf("email payload is too large")
	}
	recipientUnits := len(req.To) + len(req.CC) + len(req.BCC)
	quotaReserved := false
	reservationAt := time.Now().UTC()
	if s.usageLimiter != nil {
		limit, limitErr := s.recipientLimit(ctx, teamID)
		if limitErr != nil {
			return nil, false, fmt.Errorf("%w: %v", ErrUsageUnavailable, limitErr)
		}
		if limit > 0 {
			allowed, quotaErr := s.usageLimiter.Reserve(ctx, teamID, recipientUnits, limit)
			if quotaErr != nil {
				return nil, false, fmt.Errorf("%w: %v", ErrUsageUnavailable, quotaErr)
			}
			if !allowed {
				return nil, false, ErrDailyRecipientLimit
			}
			quotaReserved = true
		}
	}
	committed := false
	defer func() {
		if quotaReserved && !committed {
			if releaseErr := s.usageLimiter.Release(ctx, teamID, recipientUnits, reservationAt); releaseErr != nil {
				if s.logger != nil {
					s.logger.Error("failed to release daily email quota", "team_id", teamID, "error", releaseErr)
				}
			}
		}
	}()
	var idempotencyHash *string
	if idempotencyKey != "" {
		hash := hashSendRequest(req)
		idempotencyHash = &hash
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
		IdempotencyKey: func() *string {
			if idempotencyKey == "" {
				return nil
			}
			key := idempotencyKey
			return &key
		}(),
		IdempotencyHash: idempotencyHash,
		ScheduledAt:     req.ScheduledAt,
	}

	if err := s.emailRepo.Create(ctx, email); err != nil {
		if idempotencyKey != "" {
			if existing, lookupErr := s.emailRepo.GetByIdempotencyKey(ctx, teamID, idempotencyKey); lookupErr == nil && existing != nil {
				if existing.IdempotencyHash == nil || *existing.IdempotencyHash != *idempotencyHash {
					return nil, false, ErrIdempotencyConflict
				}
				return &domain.EmailResponse{ID: existing.ID.String(), Idempotent: true}, false, nil
			}
		}
		return nil, false, fmt.Errorf("failed to save email: %w", err)
	}

	var queueErr error
	if email.ScheduledAt != nil && email.ScheduledAt.After(time.Now()) {
		queueErr = s.queue.Schedule(ctx, email.ID.String(), *email.ScheduledAt)
	} else {
		queueErr = s.queue.Enqueue(ctx, email.ID.String())
	}
	if queueErr != nil {
		if statusErr := s.emailRepo.UpdateStatus(ctx, email.ID, domain.EmailStatusFailed); statusErr != nil {
			s.logger.Error("failed to mark email failed after enqueue error", "email_id", email.ID, "error", statusErr)
		}
		s.recordEvent(ctx, email.ID, "email.failed", map[string]string{"reason": "queue_unavailable"})
		return nil, false, fmt.Errorf("%w: %v", ErrQueueUnavailable, queueErr)
	}
	committed = true

	s.logger.Info("email queued", "email_id", email.ID)

	return &domain.EmailResponse{ID: email.ID.String()}, true, nil
}

func (s *EmailService) recipientLimit(ctx context.Context, teamID uuid.UUID) (int, error) {
	limit := s.dailyLimit
	if s.teamRepo == nil {
		return limit, nil
	}
	team, err := s.teamRepo.GetByID(ctx, teamID)
	if err != nil {
		return 0, err
	}
	plan := team.Plan
	if plan != domain.PlanFree && team.BillingStatus != "active" && team.BillingStatus != "trialing" {
		plan = domain.PlanFree
	}
	if planLimit, ok := s.planLimits[plan]; ok && planLimit > 0 {
		return planLimit, nil
	}
	return limit, nil
}

func hashSendRequest(req *domain.SendEmailRequest) string {
	payload, _ := json.Marshal(req)
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
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
	if offset < 0 {
		offset = 0
	}
	return s.emailRepo.List(ctx, teamID, limit, offset)
}

func (s *EmailService) GetEvents(ctx context.Context, teamID, emailID uuid.UUID) ([]domain.EmailEvent, error) {
	if _, err := s.emailRepo.GetByIDForTeam(ctx, teamID, emailID); err != nil {
		return nil, err
	}
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
	cancelled, err := s.emailRepo.CancelQueued(ctx, teamID, id)
	if err != nil {
		return fmt.Errorf("failed to cancel email: %w", err)
	}
	if !cancelled {
		return fmt.Errorf("email is no longer queued")
	}
	return nil
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
		return &EmailNotDueError{At: *email.ScheduledAt}
	}

	claimed, err := s.emailRepo.ClaimForSending(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to mark email sending: %w", err)
	}
	if !claimed {
		return fmt.Errorf("%w: email was claimed or cancelled by another worker", ErrEmailNotQueued)
	}
	s.recordEvent(ctx, id, "email.sending", nil)

	sendCtx, cancelSend := context.WithTimeout(ctx, 30*time.Second)
	providerMessageID, err := s.sender.Send(sendCtx, email)
	cancelSend()
	if err != nil {
		if domain.IsRetryableDeliveryError(err) {
			if statusErr := s.emailRepo.UpdateStatus(ctx, id, domain.EmailStatusQueued); statusErr != nil {
				return fmt.Errorf("retryable send failed: %w; failed to restore queued status: %v", err, statusErr)
			}
			s.recordEvent(ctx, id, "email.retrying", map[string]string{"reason": err.Error()})
			return fmt.Errorf("%w: %v", ErrEmailDeliveryRetryable, err)
		}
		if statusErr := s.emailRepo.UpdateStatus(ctx, id, domain.EmailStatusFailed); statusErr != nil {
			return fmt.Errorf("send failed: %w; failed to mark email failed: %v", err, statusErr)
		}
		eventID := s.recordEvent(ctx, id, "email.failed", map[string]string{"reason": err.Error()})
		s.dispatchWebhooks(ctx, email.TeamID, eventID, "email.failed", email)
		return fmt.Errorf("%w: %v", ErrEmailDeliveryFailed, err)
	}
	providerIDPersisted := true
	if strings.TrimSpace(providerMessageID) != "" {
		providerMessageID = strings.TrimSpace(providerMessageID)
		email.ProviderMessageID = &providerMessageID
		if acceptedRepo, ok := s.emailRepo.(providerAcceptedEmailRepository); ok {
			if err := acceptedRepo.MarkProviderAccepted(ctx, id, providerMessageID); err != nil {
				// SES has already accepted the message. The queue item is
				// terminal; recovery also fails stale unknown-outcome rows closed.
				s.logger.Error("failed to persist accepted provider message", "email_id", id, "error", err)
				return ErrEmailAccepted
			}
			eventID := s.recordEvent(ctx, id, "email.sent", nil)
			s.dispatchWebhooks(ctx, email.TeamID, eventID, "email.sent", email)
			return nil
		}
		if err := s.emailRepo.SetProviderMessageID(ctx, id, providerMessageID); err != nil {
			// SES has already accepted the message. Retrying here could send a duplicate,
			// so continue with the status write and return a terminal result below.
			s.logger.Error("failed to persist provider message id", "email_id", id, "error", err)
			providerIDPersisted = false
		}
	}

	if err := s.emailRepo.UpdateStatus(ctx, id, domain.EmailStatusSent); err != nil {
		// The provider has accepted the message. Returning a retryable error would
		// submit a duplicate, so the worker must acknowledge this terminal result.
		s.logger.Error("failed to mark accepted email sent", "email_id", id, "error", err)
		return ErrEmailAccepted
	}
	if !providerIDPersisted {
		return ErrEmailAccepted
	}
	eventID := s.recordEvent(ctx, id, "email.sent", nil)
	s.dispatchWebhooks(ctx, email.TeamID, eventID, "email.sent", email)

	return nil
}

func normalizeRecipients(req *domain.SendEmailRequest) []string {
	normalize := func(addresses []string) []string {
		for i, address := range addresses {
			addresses[i] = domain.NormalizeEmail(address)
		}
		return addresses
	}
	req.To = normalize(req.To)
	req.CC = normalize(req.CC)
	req.BCC = normalize(req.BCC)
	return append(append(append([]string{}, req.To...), req.CC...), req.BCC...)
}

func (s *EmailService) MarkFailedFromQueue(ctx context.Context, emailID, reason string) error {
	id, err := uuid.Parse(emailID)
	if err != nil {
		return err
	}
	if err := s.emailRepo.UpdateStatus(ctx, id, domain.EmailStatusFailed); err != nil {
		return err
	}
	s.recordEvent(ctx, id, "email.failed", map[string]string{"reason": reason})
	return nil
}

func (s *EmailService) RecoverSending(ctx context.Context) error {
	return s.emailRepo.ResetSendingToQueued(ctx)
}

func (s *EmailService) recordEvent(ctx context.Context, emailID uuid.UUID, eventName string, data any) uuid.UUID {
	eventID := uuid.New()
	dataJSON := []byte("null")
	if data != nil {
		if encoded, err := json.Marshal(data); err == nil {
			dataJSON = encoded
		}
	}
	if err := s.emailRepo.AddEvent(ctx, &domain.EmailEvent{
		ID:        eventID,
		EmailID:   emailID,
		Event:     eventName,
		Timestamp: time.Now().UTC(),
		Data:      dataJSON,
	}); err != nil {
		s.logger.Error("failed to record email event", "email_id", emailID, "event", eventName, "error", err)
	}
	return eventID
}

func (s *EmailService) dispatchWebhooks(ctx context.Context, teamID, eventID uuid.UUID, event string, payload any) {
	if s.deliveryRepo == nil || s.webhookRepo == nil {
		s.logger.Error("webhook delivery repository is not configured", "event", event)
		return
	}
	webhooks, err := s.webhookRepo.GetByEvent(ctx, teamID, event)
	if err != nil {
		s.logger.Error("failed to get webhooks", "error", err)
		return
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		s.logger.Error("failed to marshal webhook payload", "event", event, "error", err)
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
			s.logger.Error("failed to enqueue webhook delivery", "webhook_id", wh.ID, "event", event, "error", err)
		}
	}
}

func (s *EmailService) BatchSend(ctx context.Context, teamID uuid.UUID, reqs []*domain.SendEmailRequest, batchKey string) ([]*domain.EmailResponse, error) {
	if len(reqs) > 100 {
		return nil, fmt.Errorf("maximum 100 emails per batch")
	}
	batchKey = strings.TrimSpace(batchKey)
	if batchKey == "" {
		return nil, ErrBatchIdempotencyKeyRequired
	}
	if len(batchKey) > 255 || strings.ContainsAny(batchKey, "\r\n") {
		return nil, fmt.Errorf("invalid idempotency key")
	}

	var responses []*domain.EmailResponse
	for index, req := range reqs {
		resp, _, err := s.SendWithIdempotency(ctx, teamID, req, batchItemIdempotencyKey(batchKey, index))
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

func batchItemIdempotencyKey(batchKey string, index int) string {
	key := fmt.Sprintf("%s:%d", batchKey, index)
	if len(key) <= 255 {
		return key
	}
	hash := sha256.Sum256([]byte(key))
	return "batch:" + hex.EncodeToString(hash[:])
}
