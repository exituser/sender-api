package service

import (
	"testing"

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
}
