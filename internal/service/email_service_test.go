package service

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
)

type emailServiceRepoStub struct {
	domain.EmailRepository
	email        *domain.Email
	claimResult  bool
	cancelResult bool
	claimCalls   int
	cancelCalls  int
}

func (s *emailServiceRepoStub) GetByID(_ context.Context, _ uuid.UUID) (*domain.Email, error) {
	return s.email, nil
}

func (s *emailServiceRepoStub) GetByIDForTeam(_ context.Context, teamID, id uuid.UUID) (*domain.Email, error) {
	if s.email == nil || s.email.TeamID != teamID || s.email.ID != id {
		return nil, errors.New("email not found")
	}
	return s.email, nil
}

func (s *emailServiceRepoStub) ClaimForSending(context.Context, uuid.UUID) (bool, error) {
	s.claimCalls++
	return s.claimResult, nil
}

func (s *emailServiceRepoStub) CancelQueued(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	s.cancelCalls++
	return s.cancelResult, nil
}

type emailServiceSenderStub struct {
	calls int
}

type usageLimiterStub struct {
	allowed      bool
	reserveErr   error
	releaseErr   error
	reserveUnits int
	reserveLimit int
	releaseUnits int
}

func (s *usageLimiterStub) Reserve(_ context.Context, _ uuid.UUID, units, limit int) (bool, error) {
	s.reserveUnits = units
	s.reserveLimit = limit
	return s.allowed, s.reserveErr
}

func (s *usageLimiterStub) Release(_ context.Context, _ uuid.UUID, units int) error {
	s.releaseUnits = units
	return s.releaseErr
}

type emailServiceDomainRepoStub struct {
	domain.DomainRepository
	domain *domain.Domain
}

func (s *emailServiceDomainRepoStub) GetByName(context.Context, uuid.UUID, string) (*domain.Domain, error) {
	return s.domain, nil
}

type quotaEmailRepoStub struct {
	domain.EmailRepository
	createCalls       int
	updateStatusCalls int
	addEventCalls     int
	createErr         error
}

func (s *quotaEmailRepoStub) Create(_ context.Context, _ *domain.Email) error {
	s.createCalls++
	return s.createErr
}

func (s *quotaEmailRepoStub) UpdateStatus(context.Context, uuid.UUID, domain.EmailStatus) error {
	s.updateStatusCalls++
	return nil
}

func (s *quotaEmailRepoStub) AddEvent(context.Context, *domain.EmailEvent) error {
	s.addEventCalls++
	return nil
}

type quotaQueueStub struct {
	domain.EmailQueue
	enqueueCalls int
	enqueueErr   error
}

type suppressionRepoStub struct {
	domain.SuppressionRepository
	suppressions []domain.Suppression
	lookupTeam   uuid.UUID
	lookupEmails []string
	upserts      []domain.Suppression
}

func (s *suppressionRepoStub) GetByEmails(_ context.Context, teamID uuid.UUID, emails []string) ([]domain.Suppression, error) {
	s.lookupTeam = teamID
	s.lookupEmails = append([]string(nil), emails...)
	return s.suppressions, nil
}

func (s *suppressionRepoStub) Upsert(_ context.Context, suppression *domain.Suppression) error {
	s.upserts = append(s.upserts, *suppression)
	return nil
}

func (s *quotaQueueStub) Enqueue(context.Context, string) error {
	s.enqueueCalls++
	return s.enqueueErr
}

func newQuotaEmailService(repo *quotaEmailRepoStub, queue domain.EmailQueue) *EmailService {
	teamDomain := &emailServiceDomainRepoStub{domain: &domain.Domain{Status: domain.DomainStatusVerified}}
	return NewEmailService(repo, teamDomain, queue, nil, nil, nil, slog.Default())
}

func quotaRequest() *domain.SendEmailRequest {
	return &domain.SendEmailRequest{
		From:    "sender@example.com",
		To:      []string{"one@example.net", "two@example.net"},
		Subject: "hello",
		Text:    "body",
	}
}

func (s *emailServiceSenderStub) Send(context.Context, *domain.Email) (string, error) {
	s.calls++
	return "provider-message-id", nil
}

func TestEmailServiceCancelDoesNotReportSuccessAfterConcurrentClaim(t *testing.T) {
	teamID := uuid.New()
	emailID := uuid.New()
	repo := &emailServiceRepoStub{
		email: &domain.Email{ID: emailID, TeamID: teamID, Status: domain.EmailStatusQueued},
	}
	service := NewEmailService(repo, nil, nil, nil, nil, nil, nil)

	err := service.Cancel(context.Background(), teamID, emailID)
	if err == nil || err.Error() != "email is no longer queued" {
		t.Fatalf("expected cancellation race to be reported, got %v", err)
	}
	if repo.cancelCalls != 1 {
		t.Fatalf("expected one atomic cancellation attempt, got %d", repo.cancelCalls)
	}
}

func TestEmailServiceDoesNotSendAfterClaimWasLost(t *testing.T) {
	teamID := uuid.New()
	emailID := uuid.New()
	repo := &emailServiceRepoStub{
		email:       &domain.Email{ID: emailID, TeamID: teamID, Status: domain.EmailStatusQueued},
		claimResult: false,
	}
	sender := &emailServiceSenderStub{}
	service := NewEmailService(repo, nil, nil, sender, nil, nil, nil)

	err := service.ProcessFromQueue(context.Background(), emailID.String())
	if !errors.Is(err, ErrEmailNotQueued) {
		t.Fatalf("expected lost claim to be terminal, got %v", err)
	}
	if repo.claimCalls != 1 {
		t.Fatalf("expected one atomic claim attempt, got %d", repo.claimCalls)
	}
	if sender.calls != 0 {
		t.Fatalf("sender was called after claim was lost: %d", sender.calls)
	}
}

func TestEmailServiceRejectsDailyRecipientLimitBeforePersisting(t *testing.T) {
	teamID := uuid.New()
	repo := &quotaEmailRepoStub{}
	queue := &quotaQueueStub{}
	limiter := &usageLimiterStub{allowed: false}
	service := newQuotaEmailService(repo, queue)
	service.SetUsageLimiter(limiter, 10)

	_, err := service.Send(context.Background(), teamID, quotaRequest())
	if !errors.Is(err, ErrDailyRecipientLimit) {
		t.Fatalf("expected daily limit error, got %v", err)
	}
	if limiter.reserveUnits != 2 || limiter.reserveLimit != 10 {
		t.Fatalf("expected two recipients reserved against limit 10, got units=%d limit=%d", limiter.reserveUnits, limiter.reserveLimit)
	}
	if repo.createCalls != 0 || queue.enqueueCalls != 0 {
		t.Fatalf("expected rejected request not to persist or enqueue: creates=%d enqueues=%d", repo.createCalls, queue.enqueueCalls)
	}
}

func TestEmailServiceReleasesDailyRecipientLimitWhenQueueFails(t *testing.T) {
	teamID := uuid.New()
	repo := &quotaEmailRepoStub{}
	queue := &quotaQueueStub{enqueueErr: errors.New("redis down")}
	limiter := &usageLimiterStub{allowed: true}
	service := newQuotaEmailService(repo, queue)
	service.SetUsageLimiter(limiter, 10)

	_, err := service.Send(context.Background(), teamID, quotaRequest())
	if !errors.Is(err, ErrQueueUnavailable) {
		t.Fatalf("expected queue error, got %v", err)
	}
	if limiter.releaseUnits != 2 {
		t.Fatalf("expected two reserved recipients to be released, got %d", limiter.releaseUnits)
	}
	if repo.updateStatusCalls != 1 || repo.addEventCalls != 1 {
		t.Fatalf("expected failed queued email to be marked and recorded: status=%d events=%d", repo.updateStatusCalls, repo.addEventCalls)
	}
}

func TestEmailServiceKeepsDailyRecipientLimitAfterSuccessfulQueue(t *testing.T) {
	teamID := uuid.New()
	repo := &quotaEmailRepoStub{}
	queue := &quotaQueueStub{}
	limiter := &usageLimiterStub{allowed: true}
	service := newQuotaEmailService(repo, queue)
	service.SetUsageLimiter(limiter, 10)

	if _, err := service.Send(context.Background(), teamID, quotaRequest()); err != nil {
		t.Fatalf("expected email to queue, got %v", err)
	}
	if limiter.releaseUnits != 0 {
		t.Fatalf("expected successful queue to keep reservation, released %d units", limiter.releaseUnits)
	}
	if repo.createCalls != 1 || queue.enqueueCalls != 1 {
		t.Fatalf("expected one persisted and enqueued email: creates=%d enqueues=%d", repo.createCalls, queue.enqueueCalls)
	}
}

func TestEmailServiceBlocksSuppressedRecipientBeforePersisting(t *testing.T) {
	teamID := uuid.New()
	repo := &quotaEmailRepoStub{}
	queue := &quotaQueueStub{}
	suppressions := &suppressionRepoStub{suppressions: []domain.Suppression{{
		TeamID: teamID, Email: "blocked@example.net", Reason: domain.SuppressionReasonBounce,
	}}}
	service := newQuotaEmailService(repo, queue)
	service.SetSuppressionRepository(suppressions)
	req := quotaRequest()
	req.To = []string{" Blocked@Example.NET "}

	_, err := service.Send(context.Background(), teamID, req)
	if !errors.Is(err, ErrRecipientSuppressed) {
		t.Fatalf("expected suppressed recipient error, got %v", err)
	}
	if repo.createCalls != 0 || queue.enqueueCalls != 0 {
		t.Fatalf("suppressed request must not persist or enqueue: creates=%d enqueues=%d", repo.createCalls, queue.enqueueCalls)
	}
	if suppressions.lookupTeam != teamID || len(suppressions.lookupEmails) != 1 || suppressions.lookupEmails[0] != "blocked@example.net" {
		t.Fatalf("expected normalized team-scoped lookup, got team=%s emails=%v", suppressions.lookupTeam, suppressions.lookupEmails)
	}
}
