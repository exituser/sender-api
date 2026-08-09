package domain

import "testing"

func TestNormalizeEmailLowercasesAndTrims(t *testing.T) {
	if got := NormalizeEmail("  Person@Example.NET "); got != "person@example.net" {
		t.Fatalf("NormalizeEmail() = %q", got)
	}
}

func TestProviderStatusDoesNotRegressTerminalSignals(t *testing.T) {
	if ShouldApplyProviderStatus(EmailStatusComplained, EmailStatusBounced) {
		t.Fatal("bounce must not replace a complaint")
	}
	if ShouldApplyProviderStatus(EmailStatusBounced, EmailStatusDelivered) {
		t.Fatal("delivery must not replace a bounce")
	}
	if !ShouldApplyProviderStatus(EmailStatusBounced, EmailStatusComplained) {
		t.Fatal("complaint must take precedence over bounce")
	}
	if !ShouldApplyProviderStatus(EmailStatusAmbiguous, EmailStatusDelivered) {
		t.Fatal("authenticated delivery must resolve an ambiguous local outcome")
	}
}
