package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddlewareUsesConfiguredAPIKeyContext(t *testing.T) {
	SetVerifyAPIKeyContextFunc(func(context.Context, string) (*APIKeyContext, error) {
		return &APIKeyContext{
			TeamID:      "team-1",
			APIKeyID:    "key-1",
			Permissions: []string{"send"},
			Plan:        "pro",
		}, nil
	})
	defer SetVerifyAPIKeyContextFunc(nil)

	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.Header.Set("Authorization", "Bearer re_test")
	var received *Claims
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = GetClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", response.Code)
	}
	if received == nil || received.TeamID != "team-1" || received.Plan != "pro" || !HasPermission(received, "send") {
		t.Fatalf("unexpected claims: %#v", received)
	}
}

func TestAuthMiddlewareFailsClosedWithoutAPIKeyVerifier(t *testing.T) {
	SetVerifyAPIKeyContextFunc(nil)
	SetVerifyAPIKeyFunc(nil)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer re_test")
	response := httptest.NewRecorder()
	AuthMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("request should not reach handler")
	})).ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}
