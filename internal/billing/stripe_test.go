package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
)

func TestVerifyWebhookAcceptsValidSignature(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"id":"evt_test","type":"customer.subscription.updated","data":{"object":{}}}`)
	now := time.Unix(1_735_689_600, 0).UTC()
	signature := stripeTestSignature(secret, payload, now)

	client := NewStripeClient("", secret, "price_pro", "price_scale", "", "", "")
	event, err := client.VerifyWebhook(payload, signature, now)
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if event.ID != "evt_test" || event.Type != "customer.subscription.updated" {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestVerifyWebhookRejectsTamperedAndStalePayloads(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"id":"evt_test","type":"customer.subscription.updated","data":{"object":{}}}`)
	now := time.Unix(1_735_689_600, 0).UTC()
	client := NewStripeClient("", secret, "", "", "", "", "")

	valid := stripeTestSignature(secret, payload, now)
	if _, err := client.VerifyWebhook([]byte(`{"id":"evt_other","type":"customer.subscription.updated","data":{"object":{}}}`), valid, now); err == nil {
		t.Fatal("expected tampered payload to be rejected")
	}
	stale := stripeTestSignature(secret, payload, now.Add(-stripeSignatureTolerance-time.Second))
	if _, err := client.VerifyWebhook(payload, stale, now); err == nil {
		t.Fatal("expected stale signature to be rejected")
	}
}

func TestStripeAPIUsesIdempotencyForCustomerAndCheckout(t *testing.T) {
	teamID := uuid.New()
	seen := make([]*http.Request, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r)
		if r.URL.Path == "/customers" {
			_, _ = w.Write([]byte(`{"id":"cus_test"}`))
			return
		}
		if r.URL.Path == "/checkout/sessions" {
			_, _ = w.Write([]byte(`{"id":"cs_test","url":"https://checkout.example/session"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewStripeClient("sk_test", "", "price_pro", "price_scale", "https://app.example/success", "https://app.example/cancel", "https://app.example/billing")
	client.apiBaseURL = server.URL
	client.httpClient = server.Client()
	team := &domain.Team{ID: teamID, Name: "Acme"}
	if _, err := client.CreateCustomer(t.Context(), team); err != nil {
		t.Fatalf("CreateCustomer() error = %v", err)
	}
	if _, err := client.CreateCheckoutSession(t.Context(), teamID, "cus_test", domain.PlanPro); err != nil {
		t.Fatalf("CreateCheckoutSession() error = %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("received %d requests, want 2", len(seen))
	}
	if !strings.HasPrefix(seen[0].Header.Get("Idempotency-Key"), "sender-api/customer/") {
		t.Fatalf("customer request missing idempotency key: %q", seen[0].Header.Get("Idempotency-Key"))
	}
	if !strings.HasPrefix(seen[1].Header.Get("Idempotency-Key"), "sender-api/checkout/") {
		t.Fatalf("checkout request missing idempotency key: %q", seen[1].Header.Get("Idempotency-Key"))
	}
}

func stripeTestSignature(secret string, payload []byte, at time.Time) string {
	timestamp := strconv.FormatInt(at.Unix(), 10)
	hash := hmac.New(sha256.New, []byte(secret))
	_, _ = hash.Write([]byte(timestamp + "." + string(payload)))
	return "t=" + timestamp + ",v1=" + hex.EncodeToString(hash.Sum(nil))
}
