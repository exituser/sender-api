package domain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var ErrBatchRequestConflict = errors.New("batch idempotency key was already used with a different request")

type DeliveryError struct {
	Err                error
	Retryable          bool
	SMTPCode           int
	EnhancedStatusCode string
	ProviderCode       string
	OutcomeUnknown     bool
}

func (e *DeliveryError) Error() string {
	if e == nil || e.Err == nil {
		return "email delivery failed"
	}
	return e.Err.Error()
}

func (e *DeliveryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func NewDeliveryError(err error, retryable bool) error {
	return NewDeliveryErrorWithDetails(err, retryable, 0, "", "")
}

func NewDeliveryErrorWithDetails(err error, retryable bool, smtpCode int, enhancedStatusCode, providerCode string) error {
	return NewDeliveryErrorWithOutcomeDetails(err, retryable, false, smtpCode, enhancedStatusCode, providerCode)
}

func NewDeliveryErrorWithOutcomeDetails(err error, retryable, outcomeUnknown bool, smtpCode int, enhancedStatusCode, providerCode string) error {
	if err == nil {
		return fmt.Errorf("email delivery failed")
	}
	return &DeliveryError{
		Err:                err,
		Retryable:          retryable,
		SMTPCode:           smtpCode,
		EnhancedStatusCode: enhancedStatusCode,
		ProviderCode:       providerCode,
		OutcomeUnknown:     outcomeUnknown,
	}
}

func DeliveryOutcomeUnknown(err error) bool {
	if deliveryErr, ok := DeliveryErrorDetails(err); ok {
		return deliveryErr.OutcomeUnknown
	}
	return false
}

func DeliveryErrorDetails(err error) (*DeliveryError, bool) {
	var deliveryErr *DeliveryError
	if !errors.As(err, &deliveryErr) || deliveryErr == nil {
		return nil, false
	}
	return deliveryErr, true
}

func IsRetryableDeliveryError(err error) bool {
	var deliveryErr *DeliveryError
	if errors.As(err, &deliveryErr) {
		return deliveryErr.Retryable
	}
	return true
}

type EmailRepository interface {
	Create(ctx context.Context, email *Email) error
	GetByID(ctx context.Context, id uuid.UUID) (*Email, error)
	GetByIDForTeam(ctx context.Context, teamID, id uuid.UUID) (*Email, error)
	GetByIdempotencyKey(ctx context.Context, teamID uuid.UUID, key string) (*Email, error)
	GetByProviderMessageID(ctx context.Context, messageID string) (*Email, error)
	SetProviderMessageID(ctx context.Context, id uuid.UUID, messageID string) error
	ClaimForSending(ctx context.Context, id uuid.UUID) (bool, error)
	CancelQueued(ctx context.Context, teamID, id uuid.UUID) (bool, error)
	ResetSendingToQueued(ctx context.Context) error
	List(ctx context.Context, teamID uuid.UUID, limit, offset int) (*EmailListResponse, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status EmailStatus) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteForTeam(ctx context.Context, teamID, id uuid.UUID) error
	AddEvent(ctx context.Context, event *EmailEvent) error
	AddEventWithDeliveries(ctx context.Context, event *EmailEvent, deliveries []*WebhookDelivery) error
	GetEvents(ctx context.Context, emailID uuid.UUID) ([]EmailEvent, error)
	GetEventsForTeam(ctx context.Context, teamID, emailID uuid.UUID) ([]EmailEvent, error)
}

type SuppressionRepository interface {
	Upsert(ctx context.Context, suppression *Suppression) error
	GetByEmails(ctx context.Context, teamID uuid.UUID, emails []string) ([]Suppression, error)
}

type BatchRepository interface {
	Ensure(ctx context.Context, teamID uuid.UUID, key, requestHash string) (created bool, err error)
}

type DashboardRepository interface {
	GetSnapshot(ctx context.Context, teamID uuid.UUID) (*DashboardSnapshot, error)
}

type TeamRepository interface {
	Create(ctx context.Context, team *Team) error
	CreateWithOwner(ctx context.Context, team *Team, member *TeamMember) error
	GetByID(ctx context.Context, id uuid.UUID) (*Team, error)
	GetBySlug(ctx context.Context, slug string) (*Team, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]Team, error)
	Update(ctx context.Context, team *Team) error
	Delete(ctx context.Context, id uuid.UUID) error
	AddMember(ctx context.Context, member *TeamMember) error
	GetUserIDByEmail(ctx context.Context, email string) (uuid.UUID, error)
	RemoveMember(ctx context.Context, teamID, userID uuid.UUID) error
	UpdateMemberRole(ctx context.Context, teamID, userID uuid.UUID, role TeamMemberRole) error
	GetMembers(ctx context.Context, teamID uuid.UUID) ([]TeamMemberResponse, error)
	GetMember(ctx context.Context, teamID, userID uuid.UUID) (*TeamMember, error)
	CreateInvitation(ctx context.Context, invitation *TeamInvitation) error
	ListInvitations(ctx context.Context, teamID uuid.UUID) ([]TeamInvitation, error)
	RevokeInvitation(ctx context.Context, teamID, invitationID uuid.UUID) error
	AcceptInvitation(ctx context.Context, tokenHash string, userID uuid.UUID) (*TeamInvitation, error)
}

type ContactRepository interface {
	Create(ctx context.Context, contact *Contact) error
	GetByID(ctx context.Context, id uuid.UUID) (*Contact, error)
	GetByIDForTeam(ctx context.Context, teamID, id uuid.UUID) (*Contact, error)
	GetByEmail(ctx context.Context, teamID uuid.UUID, email string) (*Contact, error)
	GetUnsubscribedByEmails(ctx context.Context, teamID uuid.UUID, emails []string) ([]string, error)
	List(ctx context.Context, teamID uuid.UUID, limit, offset int) (*ContactListResponse, error)
	Update(ctx context.Context, contact *Contact) error
	UpdateForTeam(ctx context.Context, contact *Contact) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteForTeam(ctx context.Context, teamID, id uuid.UUID) error
	BulkCreate(ctx context.Context, contacts []*Contact) error
}

type DomainRepository interface {
	Create(ctx context.Context, domain *Domain) error
	GetByID(ctx context.Context, id uuid.UUID) (*Domain, error)
	GetByIDForTeam(ctx context.Context, teamID, id uuid.UUID) (*Domain, error)
	GetByName(ctx context.Context, teamID uuid.UUID, name string) (*Domain, error)
	List(ctx context.Context, teamID uuid.UUID) (*DomainListResponse, error)
	Update(ctx context.Context, domain *Domain) error
	UpdateForTeam(ctx context.Context, domain *Domain) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteForTeam(ctx context.Context, teamID, id uuid.UUID) error
	GetTeamByDomain(ctx context.Context, domainName string) (uuid.UUID, error)
}

type APIKeyRepository interface {
	Create(ctx context.Context, key *APIKey) error
	GetByID(ctx context.Context, id uuid.UUID) (*APIKey, error)
	GetByHash(ctx context.Context, hash string) (*APIKey, error)
	List(ctx context.Context, teamID uuid.UUID) ([]APIKey, error)
	Delete(ctx context.Context, id uuid.UUID) error
	UpdateLastUsed(ctx context.Context, id uuid.UUID) error
}

type InboundEmailRepository interface {
	Create(ctx context.Context, email *InboundEmail) error
	GetByID(ctx context.Context, id uuid.UUID) (*InboundEmail, error)
	GetByMessageID(ctx context.Context, teamID uuid.UUID, messageID string) (*InboundEmail, error)
	List(ctx context.Context, teamID uuid.UUID, limit, offset int) (*InboundEmailListResponse, error)
	PurgeBefore(ctx context.Context, before time.Time) (int64, error)
}

type WebhookRepository interface {
	Create(ctx context.Context, webhook *Webhook) error
	GetByID(ctx context.Context, id uuid.UUID) (*Webhook, error)
	GetByIDForTeam(ctx context.Context, teamID, id uuid.UUID) (*Webhook, error)
	List(ctx context.Context, teamID uuid.UUID) (*WebhookListResponse, error)
	Update(ctx context.Context, webhook *Webhook) error
	UpdateForTeam(ctx context.Context, webhook *Webhook) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteForTeam(ctx context.Context, teamID, id uuid.UUID) error
	GetByEvent(ctx context.Context, teamID uuid.UUID, event string) ([]Webhook, error)
}

type WebhookDeliveryRepository interface {
	CreateDelivery(ctx context.Context, delivery *WebhookDelivery) error
	ClaimDelivery(ctx context.Context) (*WebhookDelivery, error)
	MarkDelivered(ctx context.Context, id uuid.UUID) error
	MarkFailed(ctx context.Context, id uuid.UUID, reason string, retryAt time.Time) error
	RecoverStale(ctx context.Context) error
	ReplayFailed(ctx context.Context, teamID, webhookID, deliveryID uuid.UUID) error
}

type EmailSender interface {
	Send(ctx context.Context, email *Email) (string, error)
}

// SendAttemptRepository owns the PostgreSQL-authoritative state machine for
// provider submission. Queue leases are only coordination hints; every state
// mutation is fenced by AttemptID and FenceToken.
type SendAttemptRepository interface {
	ClaimSendAttempt(ctx context.Context, claim SendAttemptClaim) (bool, error)
	MarkSendStarted(ctx context.Context, claim SendAttemptClaim) (bool, error)
	MarkSendRetryable(ctx context.Context, claim SendAttemptClaim) (bool, error)
	MarkSendAmbiguous(ctx context.Context, claim SendAttemptClaim, reason string) (bool, error)
	RecoverExpiredSendAttempts(ctx context.Context) ([]string, error)
	ListQueueRecoveryPending(ctx context.Context, limit int) ([]string, error)
	MarkQueueRecoveryEnqueued(ctx context.Context, emailID uuid.UUID) error
	MarkDeadLetterFailed(ctx context.Context, emailID uuid.UUID) error
	PrepareDeadLetterReplay(ctx context.Context, teamID, emailID, replayToken uuid.UUID) (bool, error)
	CancelDeadLetterReplay(ctx context.Context, emailID, replayToken uuid.UUID) (bool, error)
}

type DeliveryPipelineRepository interface {
	CreateInboundWithOutbox(ctx context.Context, email *InboundEmail, outbox *WebhookOutboxEvent) error
	FinalizeAccepted(ctx context.Context, claim SendAttemptClaim, providerMessageID string, event *EmailEvent, outbox *WebhookOutboxEvent) (bool, error)
	FinalizeFailed(ctx context.Context, claim SendAttemptClaim, event *EmailEvent, outbox *WebhookOutboxEvent) (bool, error)
	FinalizeAmbiguous(ctx context.Context, claim SendAttemptClaim, event *EmailEvent, outbox *WebhookOutboxEvent) (bool, error)
	ReconcileAmbiguous(ctx context.Context, teamID, emailID uuid.UUID, action, providerMessageID string, event *EmailEvent, outbox *WebhookOutboxEvent) (bool, error)
	StoreProviderEvent(ctx context.Context, event *ProviderEventInbox) error
	ClaimProviderEvent(ctx context.Context, eventID *uuid.UUID) (*ProviderEventInbox, error)
	RetryProviderEvent(ctx context.Context, eventID uuid.UUID, reason string, retryAt time.Time, terminal bool) error
	ApplyProviderEvent(ctx context.Context, inboxEventID, emailID uuid.UUID, providerMessageID string, targetStatus EmailStatus, event *EmailEvent, outbox *WebhookOutboxEvent, suppressions []Suppression) error
	DispatchNextOutbox(ctx context.Context) (bool, error)
	RecoverStalePipelineWork(ctx context.Context) error
}

type EmailQueue interface {
	Enqueue(ctx context.Context, emailID string) error
	Schedule(ctx context.Context, emailID string, at time.Time) error
	Reschedule(ctx context.Context, emailID string, at time.Time) error
	PromoteScheduled(ctx context.Context) error
	Dequeue(ctx context.Context) (*QueueReceipt, error)
	Ack(ctx context.Context, receipt *QueueReceipt) error
	Requeue(ctx context.Context, receipt *QueueReceipt, countAttempt bool) error
	ListDead(ctx context.Context, limit int) ([]string, error)
	ReplayDead(ctx context.Context, emailID string) error
	Recover(ctx context.Context) error
}

type UsageLimiter interface {
	Reserve(ctx context.Context, teamID uuid.UUID, units, limit int) (bool, error)
	Release(ctx context.Context, teamID uuid.UUID, units int, reservedAt time.Time) error
}
