package service

import (
	"context"
	"errors"
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
