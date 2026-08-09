package service

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
)

type durableEmailRepoStub struct {
	domain.EmailRepository
	email              *domain.Email
	claimCalls         int
	startedCalls       int
	retryableCalls     int
	markRetryableValue bool
	recoveryPending    []string
	recoveryMarked     []uuid.UUID
	deadLetterCalls    int
	prepareReplayCalls int
	cancelReplayCalls  int
}

func (s *durableEmailRepoStub) MarkDeadLetterFailed(context.Context, uuid.UUID) error {
	s.deadLetterCalls++
	s.email.Status = domain.EmailStatusFailed
	s.email.SendAttemptState = domain.SendAttemptFailedTerminal
	s.email.SendAttemptID = nil
	s.email.SendFenceToken = nil
	s.email.SendLeaseUntil = nil
	return nil
}

func (s *durableEmailRepoStub) PrepareDeadLetterReplay(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error) {
	s.prepareReplayCalls++
	s.email.Status = domain.EmailStatusQueued
	s.email.SendAttemptState = domain.SendAttemptNone
	return true, nil
}

func (s *durableEmailRepoStub) CancelDeadLetterReplay(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	s.cancelReplayCalls++
	s.email.Status = domain.EmailStatusFailed
	s.email.SendAttemptState = domain.SendAttemptFailedTerminal
	return true, nil
}

func (s *durableEmailRepoStub) GetByID(context.Context, uuid.UUID) (*domain.Email, error) {
	copy := *s.email
	return &copy, nil
}
func (s *durableEmailRepoStub) GetByIDForTeam(context.Context, uuid.UUID, uuid.UUID) (*domain.Email, error) {
	copy := *s.email
	return &copy, nil
}
func (s *durableEmailRepoStub) AddEvent(context.Context, *domain.EmailEvent) error { return nil }
func (s *durableEmailRepoStub) ClaimSendAttempt(_ context.Context, claim domain.SendAttemptClaim) (bool, error) {
	s.claimCalls++
	s.email.SendAttemptID = &claim.AttemptID
	s.email.SendFenceToken = &claim.FenceToken
	return true, nil
}
func (s *durableEmailRepoStub) MarkSendStarted(context.Context, domain.SendAttemptClaim) (bool, error) {
	s.startedCalls++
	return true, nil
}
func (s *durableEmailRepoStub) MarkSendRetryable(context.Context, domain.SendAttemptClaim) (bool, error) {
	s.retryableCalls++
	return s.markRetryableValue, nil
}
func (s *durableEmailRepoStub) MarkSendAmbiguous(context.Context, domain.SendAttemptClaim, string) (bool, error) {
	return true, nil
}
func (s *durableEmailRepoStub) RecoverExpiredSendAttempts(context.Context) ([]string, error) {
	return nil, nil
}
func (s *durableEmailRepoStub) ListQueueRecoveryPending(context.Context, int) ([]string, error) {
	return append([]string(nil), s.recoveryPending...), nil
}
func (s *durableEmailRepoStub) MarkQueueRecoveryEnqueued(_ context.Context, id uuid.UUID) error {
	s.recoveryMarked = append(s.recoveryMarked, id)
	return nil
}

type durablePipelineStub struct {
	domain.DeliveryPipelineRepository
	acceptedCalls  int
	ambiguousCalls int
	failedCalls    int
	acceptedErr    error
	ambiguousErr   error
	reconcileCalls int
	reconcileValue bool
	reconcileErr   error
}

func (s *durablePipelineStub) FinalizeAccepted(context.Context, domain.SendAttemptClaim, string, *domain.EmailEvent, *domain.WebhookOutboxEvent) (bool, error) {
	s.acceptedCalls++
	if s.acceptedErr != nil {
		return false, s.acceptedErr
	}
	return true, nil
}
func (s *durablePipelineStub) FinalizeAmbiguous(context.Context, domain.SendAttemptClaim, *domain.EmailEvent, *domain.WebhookOutboxEvent) (bool, error) {
	s.ambiguousCalls++
	if s.ambiguousErr != nil {
		return false, s.ambiguousErr
	}
	return true, nil
}
func (s *durablePipelineStub) FinalizeFailed(context.Context, domain.SendAttemptClaim, *domain.EmailEvent, *domain.WebhookOutboxEvent) (bool, error) {
	s.failedCalls++
	return true, nil
}
func (s *durablePipelineStub) ReconcileAmbiguous(context.Context, uuid.UUID, uuid.UUID, string, string, *domain.EmailEvent, *domain.WebhookOutboxEvent) (bool, error) {
	s.reconcileCalls++
	return s.reconcileValue, s.reconcileErr
}

type durableSenderStub struct {
	calls     int
	messageID string
	err       error
}

type recoveryQueueStub struct {
	domain.EmailQueue
	enqueued []string
	failID   string
}

type deadLetterReplayQueueStub struct {
	domain.EmailQueue
	replayed []string
	err      error
}

func (s *deadLetterReplayQueueStub) ReplayDead(_ context.Context, id string) error {
	s.replayed = append(s.replayed, id)
	return s.err
}

func (s *recoveryQueueStub) Enqueue(_ context.Context, id string) error {
	s.enqueued = append(s.enqueued, id)
	if id == s.failID {
		return errors.New("redis unavailable")
	}
	return nil
}

func (s *durableSenderStub) Send(context.Context, *domain.Email) (string, error) {
	s.calls++
	return s.messageID, s.err
}

type failingSuppressionLookup struct {
	domain.SuppressionRepository
}

func (failingSuppressionLookup) GetByEmails(context.Context, uuid.UUID, []string) ([]domain.Suppression, error) {
	return nil, errors.New("database unavailable")
}

func newDurableService(repo *durableEmailRepoStub, pipeline *durablePipelineStub, sender domain.EmailSender) *EmailService {
	service := NewEmailService(repo, nil, nil, sender, nil, nil, slog.Default())
	service.SetDeliveryPipelineRepository(pipeline)
	return service
}

func queuedDurableEmail() *domain.Email {
	return &domain.Email{
		ID: uuid.New(), TeamID: uuid.New(), From: "sender@example.com",
		To: []string{"person@example.net"}, Subject: "Hello", Text: "Body",
		Status: domain.EmailStatusQueued, SendAttemptState: domain.SendAttemptNone,
	}
}

func TestDurableSendNeverCallsProviderTwiceAfterAcceptancePersistenceFailure(t *testing.T) {
	email := queuedDurableEmail()
	repo := &durableEmailRepoStub{email: email, markRetryableValue: true}
	pipeline := &durablePipelineStub{acceptedErr: errors.New("database unavailable")}
	sender := &durableSenderStub{messageID: "provider-message-id"}
	service := newDurableService(repo, pipeline, sender)

	err := service.ProcessFromQueue(context.Background(), email.ID.String())
	if !errors.Is(err, ErrEmailAccepted) {
		t.Fatalf("expected accepted-but-unpersisted result, got %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("provider called %d times; expected exactly once", sender.calls)
	}
	if pipeline.acceptedCalls != 3 || pipeline.ambiguousCalls != 1 {
		t.Fatalf("expected local retries then quarantine, got accepted=%d ambiguous=%d", pipeline.acceptedCalls, pipeline.ambiguousCalls)
	}
}

func TestDurableSendQuarantinesUnknownProviderOutcome(t *testing.T) {
	email := queuedDurableEmail()
	repo := &durableEmailRepoStub{email: email, markRetryableValue: true}
	pipeline := &durablePipelineStub{}
	sender := &durableSenderStub{err: domain.NewDeliveryErrorWithOutcomeDetails(errors.New("connection closed"), true, true, 0, "", "")}
	service := newDurableService(repo, pipeline, sender)

	err := service.ProcessFromQueue(context.Background(), email.ID.String())
	if !errors.Is(err, ErrEmailOutcomeAmbiguous) {
		t.Fatalf("expected ambiguous result, got %v", err)
	}
	if sender.calls != 1 || pipeline.ambiguousCalls != 1 || repo.retryableCalls != 0 {
		t.Fatalf("unknown outcome must be quarantined: sends=%d ambiguous=%d retries=%d", sender.calls, pipeline.ambiguousCalls, repo.retryableCalls)
	}
}

func TestDurableSendNeverRetriesUnknownOutcomeWhenQuarantinePersistenceFails(t *testing.T) {
	email := queuedDurableEmail()
	repo := &durableEmailRepoStub{email: email, markRetryableValue: true}
	pipeline := &durablePipelineStub{ambiguousErr: errors.New("database unavailable")}
	sender := &durableSenderStub{err: domain.NewDeliveryErrorWithOutcomeDetails(errors.New("connection closed"), true, true, 0, "", "")}
	service := newDurableService(repo, pipeline, sender)

	err := service.ProcessFromQueue(context.Background(), email.ID.String())
	if !errors.Is(err, ErrEmailOutcomeAmbiguous) {
		t.Fatalf("unknown outcome must remain terminal for the queue, got %v", err)
	}
	if sender.calls != 1 || pipeline.ambiguousCalls != 3 || repo.retryableCalls != 0 {
		t.Fatalf("unknown outcome must never become a provider retry: sends=%d ambiguity_writes=%d retryable=%d", sender.calls, pipeline.ambiguousCalls, repo.retryableCalls)
	}
}

func TestDurableSendRetriesPolicyLookupFailureBeforeCallingProvider(t *testing.T) {
	email := queuedDurableEmail()
	repo := &durableEmailRepoStub{email: email, markRetryableValue: true}
	pipeline := &durablePipelineStub{}
	sender := &durableSenderStub{messageID: "provider-message-id"}
	service := newDurableService(repo, pipeline, sender)
	service.SetSuppressionRepository(failingSuppressionLookup{})

	err := service.ProcessFromQueue(context.Background(), email.ID.String())
	if !errors.Is(err, ErrEmailDeliveryRetryable) {
		t.Fatalf("expected retryable policy lookup result, got %v", err)
	}
	if sender.calls != 0 || repo.retryableCalls != 1 || pipeline.failedCalls != 0 {
		t.Fatalf("provider must not be called for policy lookup failure: sends=%d retries=%d failed=%d", sender.calls, repo.retryableCalls, pipeline.failedCalls)
	}
}

func TestReconcileAmbiguousRequiresValidActionAndDurableEvidence(t *testing.T) {
	email := queuedDurableEmail()
	email.Status = domain.EmailStatusAmbiguous
	email.SendAttemptState = domain.SendAttemptAmbiguous
	repo := &durableEmailRepoStub{email: email}
	pipeline := &durablePipelineStub{reconcileValue: true}
	service := newDurableService(repo, pipeline, &durableSenderStub{})

	if _, err := service.ReconcileAmbiguous(context.Background(), email.TeamID, email.ID, domain.ReconcileEmailRequest{Action: "retry"}); !errors.Is(err, ErrInvalidDeliveryReviewAction) {
		t.Fatalf("expected invalid review action, got %v", err)
	}
	if pipeline.reconcileCalls != 0 {
		t.Fatal("invalid action reached the durable pipeline")
	}

	pipeline.reconcileErr = domain.ErrDeliveryConfirmationUnavailable
	if _, err := service.ReconcileAmbiguous(context.Background(), email.TeamID, email.ID, domain.ReconcileEmailRequest{Action: "accepted"}); !errors.Is(err, domain.ErrDeliveryConfirmationUnavailable) {
		t.Fatalf("expected missing confirmation result, got %v", err)
	}
}

func TestRecoverSendingKeepsFailedRedisEnqueuePendingAndContinues(t *testing.T) {
	first := uuid.New()
	second := uuid.New()
	repo := &durableEmailRepoStub{
		email:           queuedDurableEmail(),
		recoveryPending: []string{first.String(), second.String()},
	}
	queue := &recoveryQueueStub{failID: first.String()}
	service := NewEmailService(repo, nil, queue, nil, nil, nil, slog.Default())

	if err := service.RecoverSending(context.Background()); err == nil {
		t.Fatal("expected failed Redis enqueue to remain observable")
	}
	if len(queue.enqueued) != 2 {
		t.Fatalf("recovery stopped before later messages: enqueued=%v", queue.enqueued)
	}
	if len(repo.recoveryMarked) != 1 || repo.recoveryMarked[0] != second {
		t.Fatalf("only successfully enqueued recovery should be completed: marked=%v", repo.recoveryMarked)
	}
}

func TestDeadLetterReplayUsesDurableTransitions(t *testing.T) {
	email := queuedDurableEmail()
	repo := &durableEmailRepoStub{email: email}
	queue := &deadLetterReplayQueueStub{}
	service := NewEmailService(repo, nil, queue, nil, nil, nil, slog.Default())

	if err := service.MarkFailedFromQueue(context.Background(), email.ID.String(), "retry limit exceeded"); err != nil {
		t.Fatalf("mark dead-letter failed: %v", err)
	}
	if repo.deadLetterCalls != 1 || email.Status != domain.EmailStatusFailed || email.SendAttemptState != domain.SendAttemptFailedTerminal {
		t.Fatalf("dead-letter did not use the durable transition: email=%+v calls=%d", email, repo.deadLetterCalls)
	}
	if err := service.ReplayDeadLetter(context.Background(), email.TeamID, email.ID); err != nil {
		t.Fatalf("replay dead letter: %v", err)
	}
	if repo.prepareReplayCalls != 1 || email.Status != domain.EmailStatusQueued || email.SendAttemptState != domain.SendAttemptNone {
		t.Fatalf("replay did not prepare a fresh attempt: email=%+v calls=%d", email, repo.prepareReplayCalls)
	}
	if len(queue.replayed) != 1 || queue.replayed[0] != email.ID.String() {
		t.Fatalf("dead-letter queue was not replayed: %v", queue.replayed)
	}
}

func TestDeadLetterReplayRestoresFailureWhenQueueRejectsIt(t *testing.T) {
	email := queuedDurableEmail()
	email.Status = domain.EmailStatusFailed
	email.SendAttemptState = domain.SendAttemptFailedTerminal
	repo := &durableEmailRepoStub{email: email}
	queue := &deadLetterReplayQueueStub{err: domain.ErrDeadLetterNotFound}
	service := NewEmailService(repo, nil, queue, nil, nil, nil, slog.Default())

	err := service.ReplayDeadLetter(context.Background(), email.TeamID, email.ID)
	if !errors.Is(err, domain.ErrDeadLetterNotFound) {
		t.Fatalf("expected dead-letter membership error, got %v", err)
	}
	if repo.prepareReplayCalls != 1 || repo.cancelReplayCalls != 1 || email.Status != domain.EmailStatusFailed || email.SendAttemptState != domain.SendAttemptFailedTerminal {
		t.Fatalf("failed replay was not rolled back with its token: email=%+v prepare=%d cancel=%d", email, repo.prepareReplayCalls, repo.cancelReplayCalls)
	}
}
