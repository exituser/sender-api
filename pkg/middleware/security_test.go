package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sender-api/sender-api/internal/auth"
)

func TestSecurityHeadersAreSet(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	for header, expected := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
		"Permissions-Policy":     "camera=(), geolocation=(), microphone=()",
	} {
		if got := response.Header().Get(header); got != expected {
			t.Fatalf("expected %s=%q, got %q", header, expected, got)
		}
	}
}

func TestRateLimitScopesDoNotShareAnonymousBucket(t *testing.T) {
	ctx := context.Background()
	if got := getRateLimitKeyForScope(ctx, "stripe"); got == getRateLimitKeyForScope(ctx, "ses") {
		t.Fatalf("provider callback scopes share rate-limit key: %q", got)
	}
	teamCtx := context.WithValue(ctx, auth.ClaimsKey, &auth.Claims{TeamID: "team-1"})
	if got := getRateLimitKeyForScope(teamCtx, "api"); got != "ratelimit:api:team:team-1" {
		t.Fatalf("unexpected team rate-limit key: %q", got)
	}
}

func TestRequireTokenFailsClosed(t *testing.T) {
	handler := RequireToken("secret")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for name, token := range map[string]string{"missing": "", "wrong": "wrong"} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			request.Header.Set("X-Metrics-Token", token)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("expected unauthorized, got %d", response.Code)
			}
		})
	}

	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	request.Header.Set("X-Metrics-Token", "secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected authorized request, got %d", response.Code)
	}
}
