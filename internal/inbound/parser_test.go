package inbound

import "testing"

func TestParseRawMessage(t *testing.T) {
	message, err := ParseRawMessage("From: Sender <sender@example.com>\nTo: One <one@example.com>, two@example.com\nSubject: Test\n\nbody")
	if err != nil {
		t.Fatal(err)
	}
	if message.From != "sender@example.com" || len(message.To) != 2 || message.Subject != "Test" {
		t.Fatalf("unexpected parsed message: %#v", message)
	}
}

func TestRecipientDomain(t *testing.T) {
	if got := RecipientDomain("One <ONE@Example.COM>"); got != "example.com" {
		t.Fatalf("expected normalized domain, got %q", got)
	}
}
