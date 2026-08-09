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
	"unicode/utf8"

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
var ErrRecipientUnsubscribed = errors.New("recipient is unsubscribed")
var ErrEmailAccepted = errors.New("email was accepted by the provider but could not be fully persisted")
var ErrEmailOutcomeAmbiguous = errors.New("email provider outcome is ambiguous and will not be retried automatically")
var ErrDeliveryReconciliationUnavailable = errors.New("delivery review is temporarily unavailable")
var ErrDeliveryReviewNotNeeded = errors.New("this message no longer needs delivery review")
var ErrInvalidDeliveryReviewAction = errors.New("choose accepted or failed")

type BatchSendError struct {
	Index int
	Cause error
}

func (e *BatchSendError) Error() string {
	return fmt.Sprintf("message %d could not be queued", e.Index+1)
}

func (e *BatchSendError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type providerAcceptedEmailRepository interface {
	MarkProviderAccepted(ctx context.Context, id uuid.UUID, messageID string) error
}

type EmailService struct {
	emailRepo       domain.EmailRepository
	domainRepo      domain.DomainRepository
	queue           domain.EmailQueue
	sender          domain.EmailSender
	webhookRepo     domain.WebhookRepository
	suppressionRepo domain.SuppressionRepository
	contactRepo     domain.ContactRepository
	teamRepo        domain.TeamRepository
	batchRepo       domain.BatchRepository
	logger          *slog.Logger
	usageLimiter    domain.UsageLimiter
	dailyLimit      int
	planLimits      map[domain.Plan]int
	unsubscribe     *UnsubscribeService
	attemptRepo     domain.SendAttemptRepository
	pipelineRepo    domain.DeliveryPipelineRepository
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
	_ domain.WebhookDeliveryRepository,
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
	service := &EmailService{
		emailRepo:       emailRepo,
		domainRepo:      domainRepo,
		queue:           queue,
		sender:          sender,
		webhookRepo:     webhookRepo,
		suppressionRepo: suppressionRepo,
		logger:          logger,
	}
	service.attemptRepo, _ = emailRepo.(domain.SendAttemptRepository)
	return service
}

func (s *EmailService) SetPlanResolver(teamRepo domain.TeamRepository, freeLimit, proLimit, scaleLimit int) {
	s.teamRepo = teamRepo
	s.planLimits = map[domain.Plan]int{
		domain.PlanFree:  freeLimit,
		domain.PlanPro:   proLimit,
		domain.PlanScale: scaleLimit,
	}
}

func (s *EmailService) SetBatchRepository(repo domain.BatchRepository) {
	s.batchRepo = repo
}

// SetSuppressionRepository enables bounce and complaint suppression for
// existing composition roots without requiring constructor-call changes.
func (s *EmailService) SetSuppressionRepository(repo domain.SuppressionRepository) {
	s.suppressionRepo = repo
}

func (s *EmailService) SetContactRepository(repo domain.ContactRepository) {
	s.contactRepo = repo
}

func (s *EmailService) SetUnsubscribeService(unsubscribe *UnsubscribeService) {
	s.unsubscribe = unsubscribe
}

func (s *EmailService) SetDeliveryPipelineRepository(repo domain.DeliveryPipelineRepository) {
	s.pipelineRepo = repo
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
	category := req.Category
	if category == "" {
		category = domain.EmailCategoryTransactional
	}
	if category != domain.EmailCategoryTransactional && category != domain.EmailCategoryMarketing {
		return nil, false, fmt.Errorf("category must be transactional or marketing")
	}
	req.Category = category
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
	if !utf8.ValidString(req.Subject) || !utf8.ValidString(req.HTML) || !utf8.ValidString(req.Text) {
		return nil, false, fmt.Errorf("email content must be valid UTF-8")
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
	if category == domain.EmailCategoryMarketing {
		if len(req.To)+len(req.CC)+len(req.BCC) != 1 {
			return nil, false, fmt.Errorf("marketing emails must have exactly one recipient")
		}
		if s.unsubscribe == nil {
			return nil, false, fmt.Errorf("marketing unsubscribe is not configured")
		}
		if configuredDomain.DMARCStatus != "verified" {
			return nil, false, fmt.Errorf("marketing emails require a verified DMARC policy")
		}
	}
	requestHash := ""
	if idempotencyKey != "" {
		if len(idempotencyKey) > 255 || strings.ContainsAny(idempotencyKey, "\r\n") {
			return nil, false, fmt.Errorf("invalid idempotency key")
		}
		requestHash = hashSendRequest(req)
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
	if err := s.checkRecipientSuppression(ctx, teamID, recipients); err != nil {
		return nil, false, err
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
		if name == "" || len(name) > 100 || strings.TrimSpace(name) != name || !isRFCHeaderFieldName(name) ||
			!utf8.ValidString(name) || !utf8.ValidString(value) || strings.ContainsAny(name, "\r\n:") ||
			strings.ContainsAny(value, "\r\n") || len(value) > 998 {
			return nil, false, fmt.Errorf("invalid custom header")
		}
		switch lowerName {
		case "from", "to", "cc", "bcc", "subject", "reply-to", "sender", "date", "message-id", "return-path", "received", "content-type", "content-transfer-encoding", "content-disposition", "mime-version", "list-unsubscribe", "list-unsubscribe-post":
			return nil, false, fmt.Errorf("reserved custom header: %s", name)
		}
	}
	headers := cloneHeaders(req.Headers)
	if category == domain.EmailCategoryMarketing {
		unsubscribeURL, err := s.unsubscribe.LandingURL(teamID, req.To[0])
		if err != nil {
			return nil, false, fmt.Errorf("create unsubscribe link: %w", err)
		}
		headers["List-Unsubscribe"] = "<" + unsubscribeURL + ">"
		headers["List-Unsubscribe-Post"] = "List-Unsubscribe=One-Click"
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
		idempotencyHash = &requestHash
	}

	email := &domain.Email{
		ID:          uuid.New(),
		TeamID:      teamID,
		From:        req.From,
		To:          req.To,
		CC:          req.CC,
		BCC:         req.BCC,
		Subject:     req.Subject,
		Category:    category,
		HTML:        req.HTML,
		Text:        req.Text,
		ReplyTo:     req.ReplyTo,
		Attachments: req.Attachments,
		Status:      domain.EmailStatusQueued,
		Tags:        req.Tags,
		Metadata:    req.Metadata,
		Headers:     headers,
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

func (s *EmailService) ReconcileAmbiguous(ctx context.Context, teamID, id uuid.UUID, req domain.ReconcileEmailRequest) (*domain.Email, error) {
	if s.pipelineRepo == nil {
		return nil, ErrDeliveryReconciliationUnavailable
	}
	email, err := s.emailRepo.GetByIDForTeam(ctx, teamID, id)
	if err != nil {
		return nil, err
	}
	if email.Status != domain.EmailStatusAmbiguous || email.SendAttemptState != domain.SendAttemptAmbiguous {
		return nil, ErrDeliveryReviewNotNeeded
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	eventName := "email.failed"
	resolvedStatus := domain.EmailStatusFailed
	if action == "accepted" {
		eventName = "email.sent"
		resolvedStatus = domain.EmailStatusSent
	} else if action != "failed" {
		return nil, ErrInvalidDeliveryReviewAction
	}
	email.Status = resolvedStatus
	email.SendAttemptState = domain.SendAttemptFailedTerminal
	if action == "accepted" {
		email.SendAttemptState = domain.SendAttemptAccepted
	}
	event, outbox, err := buildDurableEvent(email, eventName, map[string]any{
		"source": "manual_reconciliation",
		"action": action,
	}, domain.RetentionOutbound)
	if err != nil {
		return nil, err
	}
	updated, err := s.pipelineRepo.ReconcileAmbiguous(ctx, teamID, id, action, req.ProviderMessageID, event, outbox)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, ErrDeliveryReviewNotNeeded
	}
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
	if s.attemptRepo != nil && s.pipelineRepo != nil {
		return s.processFromQueueDurable(ctx, emailID)
	}
	return s.processFromQueueLegacy(ctx, emailID)
}

func (s *EmailService) processFromQueueDurable(ctx context.Context, emailID string) error {
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

	claim := domain.SendAttemptClaim{
		EmailID: id, AttemptID: uuid.New(), FenceToken: uuid.New(),
		LeaseUntil: time.Now().UTC().Add(2 * time.Minute),
	}
	claimed, err := s.attemptRepo.ClaimSendAttempt(ctx, claim)
	if err != nil {
		return fmt.Errorf("claim email send attempt: %w", err)
	}
	if !claimed {
		return fmt.Errorf("%w: email was claimed or cancelled by another worker", ErrEmailNotQueued)
	}
	email.Status = domain.EmailStatusSending
	email.SendAttemptID = &claim.AttemptID
	email.SendFenceToken = &claim.FenceToken
	email.SendAttemptState = domain.SendAttemptLeased
	email.SendLeaseUntil = &claim.LeaseUntil

	if err := s.checkRecipientSuppression(ctx, email.TeamID, normalizeEmailRecipients(email)); err != nil {
		if !errors.Is(err, ErrRecipientSuppressed) && !errors.Is(err, ErrRecipientUnsubscribed) {
			requeued, stateErr := s.attemptRepo.MarkSendRetryable(ctx, claim)
			if stateErr != nil {
				return fmt.Errorf("recipient policy check unavailable: %w; restore queued state: %v", err, stateErr)
			}
			if !requeued {
				return fmt.Errorf("%w: send attempt ownership changed", ErrEmailNotQueued)
			}
			return fmt.Errorf("%w: recipient policy check unavailable: %v", ErrEmailDeliveryRetryable, err)
		}
		data := map[string]string{"reason": err.Error()}
		event, outbox, buildErr := buildDurableEvent(email, "email.failed", data, domain.RetentionOutbound)
		if buildErr != nil {
			return buildErr
		}
		persisted, persistErr := s.pipelineRepo.FinalizeFailed(ctx, claim, event, outbox)
		if persistErr != nil {
			return fmt.Errorf("recipient suppression check failed: %w; persist failure: %v", err, persistErr)
		}
		if !persisted {
			return fmt.Errorf("%w: send attempt ownership changed", ErrEmailNotQueued)
		}
		return fmt.Errorf("%w: %v", ErrEmailDeliveryFailed, err)
	}

	claim.LeaseUntil = time.Now().UTC().Add(2 * time.Minute)
	started, err := s.attemptRepo.MarkSendStarted(ctx, claim)
	if err != nil {
		return fmt.Errorf("mark provider send started: %w", err)
	}
	if !started {
		return fmt.Errorf("%w: send attempt lease changed", ErrEmailNotQueued)
	}
	email.SendAttemptState = domain.SendAttemptStarted
	email.SendLeaseUntil = &claim.LeaseUntil
	s.recordEvent(ctx, id, "email.sending", nil)

	sendCtx, cancelSend := context.WithTimeout(ctx, 30*time.Second)
	providerMessageID, sendErr := s.sender.Send(sendCtx, email)
	cancelSend()
	if sendErr != nil {
		data := deliveryErrorEventData(sendErr)
		if domain.DeliveryOutcomeUnknown(sendErr) {
			event, outbox, buildErr := buildDurableEvent(email, "email.ambiguous", data, domain.RetentionOutbound)
			if buildErr != nil {
				return fmt.Errorf("%w: build delivery review event: %v", ErrEmailOutcomeAmbiguous, buildErr)
			}
			persisted, persistErr := s.finalizeAmbiguousOutcome(ctx, claim, event, outbox)
			if persistErr != nil {
				s.logger.Error("failed to persist unknown provider outcome", "email_id", id, "attempt_id", claim.AttemptID, "error", persistErr)
				return fmt.Errorf("%w: provider outcome is unknown and local review state is pending: %v", ErrEmailOutcomeAmbiguous, sendErr)
			}
			if !persisted {
				return fmt.Errorf("%w: send attempt ownership changed", ErrEmailOutcomeAmbiguous)
			}
			return fmt.Errorf("%w: %v", ErrEmailOutcomeAmbiguous, sendErr)
		}
		if domain.IsRetryableDeliveryError(sendErr) {
			requeued, stateErr := s.attemptRepo.MarkSendRetryable(ctx, claim)
			if stateErr != nil {
				return fmt.Errorf("retryable send failed: %w; restore queued state: %v", sendErr, stateErr)
			}
			if !requeued {
				return fmt.Errorf("%w: send attempt ownership changed", ErrEmailNotQueued)
			}
			s.recordEvent(ctx, id, "email.retrying", data)
			return fmt.Errorf("%w: %v", ErrEmailDeliveryRetryable, sendErr)
		}
		event, outbox, buildErr := buildDurableEvent(email, "email.failed", data, domain.RetentionOutbound)
		if buildErr != nil {
			return buildErr
		}
		persisted, persistErr := s.pipelineRepo.FinalizeFailed(ctx, claim, event, outbox)
		if persistErr != nil {
			return fmt.Errorf("send failed: %w; persist terminal failure: %v", sendErr, persistErr)
		}
		if !persisted {
			return fmt.Errorf("%w: send attempt ownership changed", ErrEmailNotQueued)
		}
		return fmt.Errorf("%w: %v", ErrEmailDeliveryFailed, sendErr)
	}

	providerMessageID = strings.TrimSpace(providerMessageID)
	if providerMessageID == "" {
		event, outbox, buildErr := buildDurableEvent(email, "email.ambiguous", map[string]string{"reason": "provider returned no message id"}, domain.RetentionOutbound)
		if buildErr != nil {
			return fmt.Errorf("%w: build delivery review event: %v", ErrEmailOutcomeAmbiguous, buildErr)
		}
		if _, persistErr := s.finalizeAmbiguousOutcome(ctx, claim, event, outbox); persistErr != nil {
			s.logger.Error("failed to persist provider response without a receipt", "email_id", id, "attempt_id", claim.AttemptID, "error", persistErr)
			return fmt.Errorf("%w: provider returned no message id; persist ambiguity: %v", ErrEmailOutcomeAmbiguous, persistErr)
		}
		return fmt.Errorf("%w: provider returned no message id", ErrEmailOutcomeAmbiguous)
	}
	email.ProviderMessageID = &providerMessageID
	email.Status = domain.EmailStatusSent
	email.SendAttemptState = domain.SendAttemptAccepted
	event, outbox, err := buildDurableEvent(email, "email.sent", nil, domain.RetentionOutbound)
	if err != nil {
		return fmt.Errorf("build accepted email event: %w", err)
	}

	// The provider call is irreversible. Finish persistence on a short context
	// that survives worker cancellation, retrying only the local transaction.
	persistCtx, cancelPersist := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelPersist()
	var lastPersistErr error
	for attempt := 0; attempt < 3; attempt++ {
		persisted, persistErr := s.pipelineRepo.FinalizeAccepted(persistCtx, claim, providerMessageID, event, outbox)
		if persistErr == nil && persisted {
			return nil
		}
		if persistErr == nil {
			lastPersistErr = fmt.Errorf("send attempt ownership changed before acceptance commit")
			break
		}
		lastPersistErr = persistErr
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
			select {
			case <-timer.C:
			case <-persistCtx.Done():
				timer.Stop()
				attempt = 3
			}
		}
	}
	s.logger.Error("provider accepted email but local acceptance is unresolved", "email_id", id, "attempt_id", claim.AttemptID, "error", lastPersistErr)
	ambiguousEmail := *email
	ambiguousEmail.Status = domain.EmailStatusAmbiguous
	ambiguousEmail.SendAttemptState = domain.SendAttemptAmbiguous
	ambiguousEvent, ambiguousOutbox, buildErr := buildDurableEvent(&ambiguousEmail, "email.ambiguous", map[string]string{"reason": "provider acceptance needs local reconciliation"}, domain.RetentionOutbound)
	if buildErr == nil {
		ambiguityCtx, cancelAmbiguity := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		_, ambiguityErr := s.pipelineRepo.FinalizeAmbiguous(ambiguityCtx, claim, ambiguousEvent, ambiguousOutbox)
		cancelAmbiguity()
		if ambiguityErr != nil {
			s.logger.Error("failed to persist accepted email as ambiguous", "email_id", id, "attempt_id", claim.AttemptID, "error", ambiguityErr)
		}
	}
	return fmt.Errorf("%w: %v", ErrEmailAccepted, lastPersistErr)
}

func (s *EmailService) finalizeAmbiguousOutcome(ctx context.Context, claim domain.SendAttemptClaim, event *domain.EmailEvent, outbox *domain.WebhookOutboxEvent) (bool, error) {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		persisted, err := s.pipelineRepo.FinalizeAmbiguous(persistCtx, claim, event, outbox)
		if err == nil {
			return persisted, nil
		}
		lastErr = err
		if attempt == 2 {
			break
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
		select {
		case <-timer.C:
		case <-persistCtx.Done():
			timer.Stop()
			return false, errors.Join(lastErr, persistCtx.Err())
		}
	}
	return false, lastErr
}

func buildDurableEvent(email *domain.Email, eventName string, data any, retentionClass domain.RetentionClass) (*domain.EmailEvent, *domain.WebhookOutboxEvent, error) {
	eventID := uuid.New()
	dataJSON := json.RawMessage("{}")
	if data != nil {
		encoded, err := json.Marshal(data)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal email event data: %w", err)
		}
		dataJSON = encoded
	}
	payload, err := json.Marshal(email)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal email webhook payload: %w", err)
	}
	event := &domain.EmailEvent{ID: eventID, EmailID: email.ID, Event: eventName, Timestamp: time.Now().UTC(), Data: dataJSON}
	outbox := &domain.WebhookOutboxEvent{
		ID: uuid.New(), TeamID: email.TeamID, EventID: eventID, Event: eventName,
		Payload: payload, RetentionClass: retentionClass,
	}
	return event, outbox, nil
}

func (s *EmailService) processFromQueueLegacy(ctx context.Context, emailID string) error {
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
	if err := s.checkRecipientSuppression(ctx, email.TeamID, normalizeEmailRecipients(email)); err != nil {
		if statusErr := s.emailRepo.UpdateStatus(ctx, id, domain.EmailStatusFailed); statusErr != nil {
			return fmt.Errorf("recipient suppression check failed: %w; failed to mark email failed: %v", err, statusErr)
		}
		s.recordEvent(ctx, id, "email.failed", map[string]string{"reason": err.Error()})
		return fmt.Errorf("%w: %v", ErrEmailDeliveryFailed, err)
	}

	sendCtx, cancelSend := context.WithTimeout(ctx, 30*time.Second)
	providerMessageID, err := s.sender.Send(sendCtx, email)
	cancelSend()
	if err != nil {
		if domain.IsRetryableDeliveryError(err) {
			if statusErr := s.emailRepo.UpdateStatus(ctx, id, domain.EmailStatusQueued); statusErr != nil {
				return fmt.Errorf("retryable send failed: %w; failed to restore queued status: %v", err, statusErr)
			}
			s.recordEvent(ctx, id, "email.retrying", deliveryErrorEventData(err))
			return fmt.Errorf("%w: %v", ErrEmailDeliveryRetryable, err)
		}
		if statusErr := s.emailRepo.UpdateStatus(ctx, id, domain.EmailStatusFailed); statusErr != nil {
			return fmt.Errorf("send failed: %w; failed to mark email failed: %v", err, statusErr)
		}
		s.recordEventAndDispatch(ctx, email.TeamID, id, "email.failed", deliveryErrorEventData(err), email)
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
			s.recordEventAndDispatch(ctx, email.TeamID, id, "email.sent", nil, email)
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
	s.recordEventAndDispatch(ctx, email.TeamID, id, "email.sent", nil, email)

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

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return make(map[string]string)
	}
	cloned := make(map[string]string, len(headers))
	for key, value := range headers {
		cloned[key] = value
	}
	return cloned
}

func isRFCHeaderFieldName(name string) bool {
	for i := 0; i < len(name); i++ {
		char := name[i]
		if char < '!' || char > '~' || char == ':' {
			return false
		}
	}
	return name != ""
}

func deliveryErrorEventData(err error) map[string]any {
	data := map[string]any{
		"reason":    err.Error(),
		"retryable": domain.IsRetryableDeliveryError(err),
	}
	if deliveryErr, ok := domain.DeliveryErrorDetails(err); ok {
		if deliveryErr.SMTPCode != 0 {
			data["smtp_code"] = deliveryErr.SMTPCode
		}
		if deliveryErr.EnhancedStatusCode != "" {
			data["enhanced_status_code"] = deliveryErr.EnhancedStatusCode
		}
		if deliveryErr.ProviderCode != "" {
			data["provider_code"] = deliveryErr.ProviderCode
		}
	}
	return data
}

func normalizeEmailRecipients(email *domain.Email) []string {
	recipients := append(append(append([]string{}, email.To...), email.CC...), email.BCC...)
	for index, recipient := range recipients {
		recipients[index] = domain.NormalizeEmail(recipient)
	}
	return recipients
}

func (s *EmailService) checkRecipientSuppression(ctx context.Context, teamID uuid.UUID, recipients []string) error {
	if s.suppressionRepo != nil {
		suppressions, err := s.suppressionRepo.GetByEmails(ctx, teamID, recipients)
		if err != nil {
			return fmt.Errorf("check recipient suppressions: %w", err)
		}
		if len(suppressions) > 0 {
			return fmt.Errorf("%w: %s (%s)", ErrRecipientSuppressed, suppressions[0].Email, suppressions[0].Reason)
		}
	}
	if s.contactRepo != nil {
		unsubscribed, err := s.contactRepo.GetUnsubscribedByEmails(ctx, teamID, recipients)
		if err != nil {
			return fmt.Errorf("check contact subscriptions: %w", err)
		}
		if len(unsubscribed) > 0 {
			return fmt.Errorf("%w: %s", ErrRecipientUnsubscribed, unsubscribed[0])
		}
	}
	return nil
}

func (s *EmailService) MarkFailedFromQueue(ctx context.Context, emailID, reason string) error {
	id, err := uuid.Parse(emailID)
	if err != nil {
		return err
	}
	if s.attemptRepo != nil {
		err = s.attemptRepo.MarkDeadLetterFailed(ctx, id)
	} else {
		err = s.emailRepo.UpdateStatus(ctx, id, domain.EmailStatusFailed)
	}
	if err != nil {
		return err
	}
	s.recordEvent(ctx, id, "email.failed", map[string]string{"reason": reason})
	return nil
}

func (s *EmailService) RecoverSending(ctx context.Context) error {
	if s.attemptRepo != nil {
		_, err := s.attemptRepo.RecoverExpiredSendAttempts(ctx)
		if err != nil {
			return err
		}
		ids, err := s.attemptRepo.ListQueueRecoveryPending(ctx, 5000)
		if err != nil {
			return fmt.Errorf("list pending queue recovery: %w", err)
		}
		var recoveryErrors []error
		for _, id := range ids {
			if err := s.queue.Enqueue(ctx, id); err != nil {
				recoveryErrors = append(recoveryErrors, fmt.Errorf("enqueue recovered send attempt %s: %w", id, err))
				continue
			}
			emailID, parseErr := uuid.Parse(id)
			if parseErr != nil {
				recoveryErrors = append(recoveryErrors, fmt.Errorf("parse recovered email id %q: %w", id, parseErr))
				continue
			}
			if err := s.attemptRepo.MarkQueueRecoveryEnqueued(ctx, emailID); err != nil {
				recoveryErrors = append(recoveryErrors, fmt.Errorf("complete queue recovery %s: %w", id, err))
			}
		}
		return errors.Join(recoveryErrors...)
	}
	return s.emailRepo.ResetSendingToQueued(ctx)
}

func (s *EmailService) ListDeadLetters(ctx context.Context, teamID uuid.UUID, limit int) ([]domain.DeadLetter, error) {
	ids, err := s.queue.ListDead(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]domain.DeadLetter, 0, len(ids))
	for _, rawID := range ids {
		id, parseErr := uuid.Parse(rawID)
		if parseErr != nil {
			continue
		}
		email, getErr := s.emailRepo.GetByIDForTeam(ctx, teamID, id)
		if getErr != nil {
			continue
		}
		if email.Status != domain.EmailStatusFailed {
			continue
		}
		result = append(result, domain.DeadLetter{ID: email.ID.String(), Status: email.Status})
	}
	return result, nil
}

func (s *EmailService) ReplayDeadLetter(ctx context.Context, teamID, emailID uuid.UUID) error {
	email, err := s.emailRepo.GetByIDForTeam(ctx, teamID, emailID)
	if err != nil {
		return err
	}
	if email.Status == domain.EmailStatusAmbiguous || email.SendAttemptState == domain.SendAttemptAmbiguous {
		return fmt.Errorf("ambiguous provider outcomes cannot be resent; reconcile the provider result instead")
	}
	replayToken := uuid.New()
	if s.attemptRepo != nil {
		prepared, prepareErr := s.attemptRepo.PrepareDeadLetterReplay(ctx, teamID, emailID, replayToken)
		if prepareErr != nil {
			return prepareErr
		}
		if !prepared {
			return fmt.Errorf("email is not a failed dead-letter candidate")
		}
	} else if err := s.emailRepo.UpdateStatus(ctx, emailID, domain.EmailStatusQueued); err != nil {
		return err
	}
	if err := s.queue.ReplayDead(ctx, emailID.String()); err != nil {
		var rollbackErr error
		if s.attemptRepo != nil {
			var cancelled bool
			cancelled, rollbackErr = s.attemptRepo.CancelDeadLetterReplay(ctx, emailID, replayToken)
			if rollbackErr == nil && !cancelled {
				rollbackErr = fmt.Errorf("replay ownership changed")
			}
		} else {
			rollbackErr = s.emailRepo.UpdateStatus(ctx, emailID, domain.EmailStatusFailed)
		}
		if rollbackErr != nil {
			return fmt.Errorf("replay dead letter: %w; restore failed status: %v", err, rollbackErr)
		}
		return err
	}
	s.recordEvent(ctx, emailID, "email.requeued", map[string]string{"reason": "manual_dead_letter_replay"})
	return nil
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
		Data:      json.RawMessage(dataJSON),
	}); err != nil {
		s.logger.Error("failed to record email event", "email_id", emailID, "event", eventName, "error", err)
	}
	return eventID
}

func (s *EmailService) recordEventAndDispatch(ctx context.Context, teamID, emailID uuid.UUID, eventName string, data, payload any) uuid.UUID {
	eventID := uuid.New()
	if s.webhookRepo == nil {
		return s.recordEvent(ctx, emailID, eventName, data)
	}
	dataJSON := []byte("null")
	if data != nil {
		if encoded, err := json.Marshal(data); err == nil {
			dataJSON = encoded
		}
	}
	event := &domain.EmailEvent{
		ID: eventID, EmailID: emailID, Event: eventName, Timestamp: time.Now().UTC(), Data: dataJSON,
	}
	webhooks, err := s.webhookRepo.GetByEvent(ctx, teamID, eventName)
	if err != nil {
		s.logger.Error("failed to get webhooks", "error", err, "event", eventName)
		return s.recordEvent(ctx, emailID, eventName, data)
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		s.logger.Error("failed to marshal webhook payload", "event", eventName, "error", err)
		return s.recordEvent(ctx, emailID, eventName, data)
	}
	deliveries := make([]*domain.WebhookDelivery, 0, len(webhooks))
	for _, webhook := range webhooks {
		deliveries = append(deliveries, &domain.WebhookDelivery{
			ID: uuid.New(), WebhookID: webhook.ID, EventID: eventID, Event: eventName, Payload: payloadJSON,
		})
	}
	if err := s.emailRepo.AddEventWithDeliveries(ctx, event, deliveries); err != nil {
		s.logger.Error("failed to record event and enqueue webhook deliveries", "email_id", emailID, "event", eventName, "error", err)
	}
	return eventID
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
	if s.batchRepo != nil {
		_, err := s.batchRepo.Ensure(ctx, teamID, batchKey, hashBatchRequests(reqs))
		if err != nil {
			if errors.Is(err, domain.ErrBatchRequestConflict) {
				return nil, ErrIdempotencyConflict
			}
			return nil, fmt.Errorf("check batch idempotency: %w", err)
		}
	}

	var responses []*domain.EmailResponse
	for index, req := range reqs {
		resp, _, err := s.SendWithIdempotency(ctx, teamID, req, batchItemIdempotencyKey(batchKey, index))
		if err != nil {
			return responses, &BatchSendError{Index: index, Cause: err}
		}
		responses = append(responses, resp)
	}
	return responses, nil
}

func hashBatchRequests(reqs []*domain.SendEmailRequest) string {
	payload, _ := json.Marshal(reqs)
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}

func batchItemIdempotencyKey(batchKey string, index int) string {
	key := fmt.Sprintf("%s:%d", batchKey, index)
	if len(key) <= 255 {
		return key
	}
	hash := sha256.Sum256([]byte(key))
	return "batch:" + hex.EncodeToString(hash[:])
}
