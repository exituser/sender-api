package worker

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/internal/service"
)

type acceptedEmailRepoStub struct {
	domain.EmailRepository
	email             *domain.Email
	setProviderCalls  int
	updateStatusCalls int
}

func (s *acceptedEmailRepoStub) GetByID(context.Context, uuid.UUID) (*domain.Email, error) {
	return s.email, nil
}

func (s *acceptedEmailRepoStub) ClaimForSending(context.Context, uuid.UUID) (bool, error) {
	return true, nil
}

func (s *acceptedEmailRepoStub) SetProviderMessageID(context.Context, uuid.UUID, string) error {
	s.setProviderCalls++
	return nil
}

func (s *acceptedEmailRepoStub) UpdateStatus(context.Context, uuid.UUID, domain.EmailStatus) error {
	s.updateStatusCalls++
	return errors.New("database unavailable")
}

func (s *acceptedEmailRepoStub) AddEvent(context.Context, *domain.EmailEvent) error {
	return nil
}

type acceptedSenderStub struct{ calls int }

func (s *acceptedSenderStub) Send(context.Context, *domain.Email) (string, error) {
	s.calls++
	return "ses-message-id", nil
}

type acceptedQueueStub struct {
	domain.EmailQueue
	emailID      string
	ackCalls     int
	requeueCalls int
}

func (s *acceptedQueueStub) Dequeue(context.Context) (string, error) { return s.emailID, nil }
func (s *acceptedQueueStub) Ack(context.Context, string) error {
	s.ackCalls++
	return nil
}
func (s *acceptedQueueStub) Requeue(context.Context, string, bool) error {
	s.requeueCalls++
	return nil
}

func TestEmailWorkerAcknowledgesProviderAcceptedEmailWhenStatusPersistenceFails(t *testing.T) {
	emailID := uuid.New()
	repo := &acceptedEmailRepoStub{email: &domain.Email{ID: emailID, TeamID: uuid.New(), Status: domain.EmailStatusQueued}}
	sender := &acceptedSenderStub{}
	queue := &acceptedQueueStub{emailID: emailID.String()}
	emailService := service.NewEmailService(repo, nil, nil, sender, nil, nil, slog.Default())
	worker := NewEmailWorker(emailService, queue, slog.Default(), 0)

	worker.processNext(context.Background())

	if sender.calls != 1 || repo.setProviderCalls != 1 || repo.updateStatusCalls != 1 {
		t.Fatalf("expected one accepted send and one persistence attempt, sender=%d provider_id=%d status=%d", sender.calls, repo.setProviderCalls, repo.updateStatusCalls)
	}
	if queue.ackCalls != 1 || queue.requeueCalls != 0 {
		t.Fatalf("accepted email must be acknowledged without retry: ack=%d requeue=%d", queue.ackCalls, queue.requeueCalls)
	}
}
