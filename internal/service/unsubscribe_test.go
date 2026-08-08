package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
)

type unsubscribeContactStub struct {
	domain.ContactRepository
	teamID      uuid.UUID
	email       string
	subscribed  bool
	updateCalls int
}

func (s *unsubscribeContactStub) SetSubscribedByEmail(_ context.Context, teamID uuid.UUID, email string, subscribed bool) (bool, error) {
	s.teamID = teamID
	s.email = email
	s.subscribed = subscribed
	s.updateCalls++
	return true, nil
}

type unsubscribeSuppressionStub struct {
	domain.SuppressionRepository
	suppression *domain.Suppression
}

func (s *unsubscribeSuppressionStub) Upsert(_ context.Context, suppression *domain.Suppression) error {
	s.suppression = suppression
	return nil
}

func TestUnsubscribeServiceConsumesOpaqueToken(t *testing.T) {
	teamID := uuid.New()
	contacts := &unsubscribeContactStub{}
	suppressions := &unsubscribeSuppressionStub{}
	signer, err := NewUnsubscribeSigner("0123456789abcdef0123456789abcdef", "https://api.example.test")
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	service := NewUnsubscribeService(signer, contacts, suppressions)
	token, err := signer.Token(teamID, "Person@Example.NET")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	if err := service.Unsubscribe(context.Background(), token); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	if contacts.updateCalls != 1 || contacts.teamID != teamID || contacts.email != "person@example.net" || contacts.subscribed {
		t.Fatalf("unexpected contact update: %+v", contacts)
	}
	if suppressions.suppression == nil || suppressions.suppression.Reason != domain.SuppressionReasonUnsubscribe {
		t.Fatalf("expected unsubscribe suppression, got %+v", suppressions.suppression)
	}
}

func TestUnsubscribeServiceRejectsTamperedToken(t *testing.T) {
	signer, err := NewUnsubscribeSigner("0123456789abcdef0123456789abcdef", "https://api.example.test")
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	service := NewUnsubscribeService(signer, &unsubscribeContactStub{}, &unsubscribeSuppressionStub{})
	if err := service.Unsubscribe(context.Background(), "tampered"); err != ErrInvalidUnsubscribeToken {
		t.Fatalf("expected invalid token, got %v", err)
	}
}
