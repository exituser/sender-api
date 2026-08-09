package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
)

func TestProviderEventDetails(t *testing.T) {
	tests := []struct {
		event  string
		name   string
		status domain.EmailStatus
	}{
		{event: "Delivery", name: "email.delivered", status: domain.EmailStatusDelivered},
		{event: "Rendering Failure", name: "email.failed", status: domain.EmailStatusFailed},
		{event: "Click", name: "email.clicked", status: domain.EmailStatusClicked},
	}
	for _, test := range tests {
		name, status, hasStatus := providerEventDetails(test.event)
		if name != test.name || status != test.status || !hasStatus {
			t.Fatalf("unexpected mapping for %q: %q %q %t", test.event, name, status, hasStatus)
		}
	}
}

type providerEventEmailRepoStub struct {
	domain.EmailRepository
	email *domain.Email
}

func (s *providerEventEmailRepoStub) GetByProviderMessageID(context.Context, string) (*domain.Email, error) {
	return s.email, nil
}

func (s *providerEventEmailRepoStub) UpdateStatus(context.Context, uuid.UUID, domain.EmailStatus) error {
	return nil
}

func (s *providerEventEmailRepoStub) AddEvent(context.Context, *domain.EmailEvent) error {
	return nil
}

type providerEventSuppressionRepoStub struct {
	domain.SuppressionRepository
	upserts []domain.Suppression
}

func (s *providerEventSuppressionRepoStub) Upsert(_ context.Context, suppression *domain.Suppression) error {
	s.upserts = append(s.upserts, *suppression)
	return nil
}

func TestProviderBounceCreatesNormalizedTeamScopedSuppressions(t *testing.T) {
	teamID := uuid.New()
	repo := &providerEventEmailRepoStub{email: &domain.Email{ID: uuid.New(), TeamID: teamID, Status: domain.EmailStatusSent}}
	suppressions := &providerEventSuppressionRepoStub{}
	service := NewEmailService(repo, nil, nil, nil, nil, nil, slog.Default(), suppressions)
	payload := json.RawMessage(`{"bounce":{"bouncedRecipients":[{"emailAddress":" Bounced@Example.NET "}]}}`)

	if err := service.ProcessProviderEvent(context.Background(), "ses-message-id", "Bounce", payload, uuid.New()); err != nil {
		t.Fatalf("process bounce: %v", err)
	}
	if len(suppressions.upserts) != 1 {
		t.Fatalf("expected one suppression, got %d", len(suppressions.upserts))
	}
	suppression := suppressions.upserts[0]
	if suppression.TeamID != teamID || suppression.Email != "bounced@example.net" || suppression.Reason != domain.SuppressionReasonBounce {
		t.Fatalf("unexpected suppression: %+v", suppression)
	}
}

func TestProviderComplaintCreatesTeamScopedSuppression(t *testing.T) {
	teamID := uuid.New()
	repo := &providerEventEmailRepoStub{email: &domain.Email{ID: uuid.New(), TeamID: teamID, Status: domain.EmailStatusSent}}
	suppressions := &providerEventSuppressionRepoStub{}
	service := NewEmailService(repo, nil, nil, nil, nil, nil, slog.Default(), suppressions)
	payload := json.RawMessage(`{"complaint":{"complainedRecipients":[{"emailAddress":"complained@example.net"}]}}`)

	if err := service.ProcessProviderEvent(context.Background(), "ses-message-id", "Complaint", payload, uuid.New()); err != nil {
		t.Fatalf("process complaint: %v", err)
	}
	if len(suppressions.upserts) != 1 || suppressions.upserts[0].TeamID != teamID || suppressions.upserts[0].Reason != domain.SuppressionReasonComplaint {
		t.Fatalf("expected one team-scoped complaint suppression, got %+v", suppressions.upserts)
	}
}

func TestProviderStatusDoesNotRegress(t *testing.T) {
	if shouldApplyProviderStatus(domain.EmailStatusDelivered, domain.EmailStatusSent) {
		t.Fatal("delivery status must not regress to sent")
	}
	if !shouldApplyProviderStatus(domain.EmailStatusDelivered, domain.EmailStatusBounced) {
		t.Fatal("bounce must remain observable after delivery")
	}
	if shouldApplyProviderStatus(domain.EmailStatusBounced, domain.EmailStatusOpened) {
		t.Fatal("open must not override a bounce")
	}
	if shouldApplyProviderStatus(domain.EmailStatusFailed, domain.EmailStatusDelivered) {
		t.Fatal("a positive event must not resurrect a terminal failure")
	}
	if !shouldApplyProviderStatus(domain.EmailStatusAmbiguous, domain.EmailStatusSent) {
		t.Fatal("verified provider evidence should resolve an ambiguous send")
	}
}

func TestProviderRetryTerminalizesDeterministicMismatchAndStaleEvents(t *testing.T) {
	now := time.Now().UTC()
	inbox := &domain.ProviderEventInbox{CreatedAt: now.Add(-time.Hour)}
	if !providerRetryIsTerminal(inbox, domain.ErrProviderCorrelationMismatch, now) {
		t.Fatal("deterministic provider correlation mismatch must not retry forever")
	}
	if providerRetryIsTerminal(inbox, errors.New("database unavailable"), now) {
		t.Fatal("fresh transient provider event failure should remain retryable")
	}
	inbox.CreatedAt = now.Add(-7 * 24 * time.Hour)
	if !providerRetryIsTerminal(inbox, errors.New("database unavailable"), now) {
		t.Fatal("stale provider event must eventually leave the retry loop")
	}
}
