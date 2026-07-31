package validator

import (
	"net"
	"testing"
)

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

func TestCanonicalEmailPreservesLocalPartAndNormalizesDomain(t *testing.T) {
	if got := CanonicalEmail("User@EXAMPLE.COM"); got != "User@example.com" {
		t.Fatalf("expected canonical email, got %q", got)
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

func TestIsValidHTTPSURLRequiresHTTPS(t *testing.T) {
	if IsValidHTTPSURL("http://example.com/webhook") {
		t.Fatal("expected HTTPS validator to reject HTTP")
	}
	if !IsValidHTTPSURL("https://example.com/webhook") {
		t.Fatal("expected HTTPS validator to accept a public HTTPS endpoint")
	}
}

func TestIsPrivateIPRejectsUnroutableIPv4Range(t *testing.T) {
	if !IsPrivateIP(net.ParseIP("0.1.2.3")) {
		t.Fatal("expected 0.0.0.0/8 to be rejected")
	}
}
