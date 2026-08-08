package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
)

type marketingEmailRepoStub struct {
	domain.EmailRepository
	email *domain.Email
}

func (s *marketingEmailRepoStub) Create(_ context.Context, email *domain.Email) error {
	s.email = email
	return nil
}

func TestMarketingEmailAddsOneClickUnsubscribeHeaders(t *testing.T) {
	repo := &marketingEmailRepoStub{}
	queue := &quotaQueueStub{}
	teamID := uuid.New()
	domainRepo := &emailServiceDomainRepoStub{domain: &domain.Domain{
		Status:      domain.DomainStatusVerified,
		DMARCStatus: "verified",
	}}
	signer, err := NewUnsubscribeSigner("0123456789abcdef0123456789abcdef", "https://api.example.test")
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	service := NewEmailService(repo, domainRepo, queue, nil, nil, nil, nil)
	service.SetUnsubscribeService(NewUnsubscribeService(signer, nil, nil))

	_, err = service.Send(context.Background(), teamID, &domain.SendEmailRequest{
		From:     "sender@example.com",
		To:       []string{"recipient@example.net"},
		Subject:  "Monthly update",
		Text:     "Hello",
		Category: domain.EmailCategoryMarketing,
	})
	if err != nil {
		t.Fatalf("send marketing email: %v", err)
	}
	if repo.email == nil {
		t.Fatal("expected email to be persisted")
	}
	if repo.email.Category != domain.EmailCategoryMarketing {
		t.Fatalf("expected marketing category, got %q", repo.email.Category)
	}
	if repo.email.Headers["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" || repo.email.Headers["List-Unsubscribe"] == "" {
		t.Fatalf("expected RFC unsubscribe headers, got %+v", repo.email.Headers)
	}
	if queue.enqueueCalls != 1 {
		t.Fatalf("expected one enqueue, got %d", queue.enqueueCalls)
	}
}

func TestMarketingEmailRequiresDMARC(t *testing.T) {
	service := NewEmailService(&marketingEmailRepoStub{}, &emailServiceDomainRepoStub{domain: &domain.Domain{
		Status: domain.DomainStatusVerified,
	}}, &quotaQueueStub{}, nil, nil, nil, nil)
	signer, err := NewUnsubscribeSigner("0123456789abcdef0123456789abcdef", "https://api.example.test")
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	service.SetUnsubscribeService(NewUnsubscribeService(signer, nil, nil))
	_, err = service.Send(context.Background(), uuid.New(), &domain.SendEmailRequest{
		From:     "sender@example.com",
		To:       []string{"recipient@example.net"},
		Subject:  "Monthly update",
		Text:     "Hello",
		Category: domain.EmailCategoryMarketing,
	})
	if err == nil || err.Error() != "marketing emails require a verified DMARC policy" {
		t.Fatalf("expected DMARC rejection, got %v", err)
	}
}
