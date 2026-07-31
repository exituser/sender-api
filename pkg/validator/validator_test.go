package validator

import "testing"

func TestIsValidEmail(t *testing.T) {
	if !IsValidEmail("person@example.com") {
		t.Fatal("expected valid email")
	}
	if IsValidEmail("Person <person@example.com>") || IsValidEmail("person@example.com\r\nBcc: attacker@example.com") {
		t.Fatal("expected unsafe email to be rejected")
	}
}

func TestIsValidDomain(t *testing.T) {
	if !IsValidDomain("example.com") {
		t.Fatal("expected valid domain")
	}
	if IsValidDomain("-example.com") || IsValidDomain("localhost") {
		t.Fatal("expected invalid domain to be rejected")
	}
}

func TestIsValidURLRejectsPrivateTargets(t *testing.T) {
	if !IsValidURL("https://example.com/webhook") {
		t.Fatal("expected public URL")
	}
	if IsValidURL("http://127.0.0.1:8080/internal") || IsValidURL("http://localhost/internal") {
		t.Fatal("expected private URL to be rejected")
	}
}
