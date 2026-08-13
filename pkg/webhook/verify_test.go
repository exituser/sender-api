package webhook

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestVerifySignatureRoundTripAndTamperResistance(t *testing.T) {
	payload := []byte(`{"event":"email.delivered","id":"delivery-id"}`)
	secret := "webhook-secret"

	signature := SignPayload(payload, secret)
	if !VerifySignature(payload, signature, secret) {
		t.Fatal("expected the generated webhook signature to verify")
	}
	if VerifySignature([]byte(`{"event":"email.failed"}`), signature, secret) {
		t.Fatal("tampered webhook payload unexpectedly verified")
	}
	if VerifySignature(payload, signature, "different-secret") {
		t.Fatal("webhook signature verified with a different secret")
	}
}

func TestSendWebhookWithIDPolicyRejectsUnsafeURLs(t *testing.T) {
	for _, rawURL := range []string{
		"http://127.0.0.1:8080/webhook",
		"http://localhost:8080/webhook",
		"http://example.com/webhook",
	} {
		t.Run(rawURL, func(t *testing.T) {
			err := SendWebhookWithIDPolicy(
				t.Context(),
				uuid.Nil,
				rawURL,
				"secret",
				"email.delivered",
				map[string]string{"status": "delivered"},
				true,
			)
			if err == nil {
				t.Fatal("expected unsafe or non-HTTPS webhook URL to be rejected")
			}
			if !strings.Contains(err.Error(), "unsafe webhook url") && !strings.Contains(err.Error(), "must use HTTPS") {
				t.Fatalf("unexpected URL policy error: %v", err)
			}
		})
	}
}

func TestSafeDialContextRejectsPrivateAddresses(t *testing.T) {
	_, err := safeDialContext(context.Background(), "tcp", "127.0.0.1:1")
	if err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("expected private-address rejection, got %v", err)
	}

	_, err = safeDialContext(context.Background(), "tcp", "[::1]:1")
	if err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("expected IPv6 loopback rejection, got %v", err)
	}
}

func TestSendWebhookWithIDPolicyHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := SendWebhookWithIDPolicy(ctx, uuid.Nil, "https://example.com/webhook", "secret", "email.delivered", nil, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected cancelled webhook result: %v", err)
	}
}
