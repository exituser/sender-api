package service

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
)

type contactServiceStub struct {
	domain.ContactRepository
	created []*domain.Contact
	byEmail *domain.Contact
	updated *domain.Contact
}

func (s *contactServiceStub) GetByEmail(context.Context, uuid.UUID, string) (*domain.Contact, error) {
	return s.byEmail, nil
}

func (s *contactServiceStub) Create(_ context.Context, contact *domain.Contact) error {
	s.created = append(s.created, contact)
	return nil
}

func (s *contactServiceStub) BulkCreate(_ context.Context, contacts []*domain.Contact) error {
	s.created = append(s.created, contacts...)
	return nil
}

func (s *contactServiceStub) GetByIDForTeam(context.Context, uuid.UUID, uuid.UUID) (*domain.Contact, error) {
	return s.byEmail, nil
}

func (s *contactServiceStub) UpdateForTeam(_ context.Context, contact *domain.Contact) error {
	s.updated = contact
	return nil
}

func TestContactServiceCanonicalizesEmailsAcrossWrites(t *testing.T) {
	teamID := uuid.New()
	stub := &contactServiceStub{}
	service := NewContactService(stub, slog.Default())

	created, err := service.Create(context.Background(), teamID, &domain.CreateContactRequest{Email: "User@EXAMPLE.COM"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Email != "user@example.com" {
		t.Fatalf("Create() email = %q", created.Email)
	}

	count, err := service.ImportCSV(context.Background(), teamID, []*domain.CreateContactRequest{{Email: "BULK@Example.COM"}})
	if err != nil || count != 1 || stub.created[len(stub.created)-1].Email != "bulk@example.com" {
		t.Fatalf("ImportCSV() count=%d error=%v contacts=%+v", count, err, stub.created)
	}

	stub.byEmail = &domain.Contact{ID: uuid.New(), TeamID: teamID, Email: "old@example.com", Subscribed: true}
	updatedEmail := "UPDATE@EXAMPLE.COM"
	if _, err := service.Update(context.Background(), teamID, stub.byEmail.ID, &domain.UpdateContactRequest{Email: &updatedEmail}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if stub.updated == nil {
		t.Fatal("Update() did not call repository")
	}
	if stub.updated.Email != "update@example.com" {
		t.Fatalf("Update() email = %q", stub.updated.Email)
	}
}
