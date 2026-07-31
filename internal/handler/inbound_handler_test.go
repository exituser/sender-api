package handler

import "testing"

func TestExtractInboundHeadersParsesDisplayNamesAndMultipleRecipients(t *testing.T) {
	from, to, subject := extractInboundHeaders("From: Sender <sender@example.com>\nTo: One <one@example.com>, two@example.com\nSubject: Test\n\nbody")

	if from != "sender@example.com" {
		t.Fatalf("expected sender address, got %q", from)
	}
	if len(to) != 2 || to[0] != "one@example.com" || to[1] != "two@example.com" {
		t.Fatalf("unexpected recipients: %#v", to)
	}
	if subject != "Test" {
		t.Fatalf("expected subject, got %q", subject)
	}
}

func TestExtractInboundHeadersRejectsMalformedMessage(t *testing.T) {
	from, to, subject := extractInboundHeaders("not an email message")
	if from != "" || len(to) != 0 || subject != "" {
		t.Fatalf("expected malformed message to be rejected: %q %#v %q", from, to, subject)
	}
}

func TestInboundRecipientDomainNormalizesAddress(t *testing.T) {
	if got := inboundRecipientDomain("One <ONE@Example.COM>"); got != "example.com" {
		t.Fatalf("expected normalized recipient domain, got %q", got)
	}
	if got := inboundRecipientDomain("invalid"); got != "" {
		t.Fatalf("expected invalid recipient to be rejected, got %q", got)
	}
}
