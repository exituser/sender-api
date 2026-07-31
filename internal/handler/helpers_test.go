package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/auth"
)

func TestGetTeamIDRejectsMissingTeamContext(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = request.WithContext(context.WithValue(request.Context(), auth.ClaimsKey, &auth.Claims{UserID: "user-1", Role: "user"}))
	response := httptest.NewRecorder()

	_, _, ok := getTeamID(response, request)
	if ok {
		t.Fatal("expected missing team context to be rejected")
	}
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
}

func TestGetTeamIDParsesAuthenticatedTeam(t *testing.T) {
	teamID := uuid.New()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = request.WithContext(context.WithValue(request.Context(), auth.ClaimsKey, &auth.Claims{TeamID: teamID.String(), Role: "member"}))
	response := httptest.NewRecorder()

	_, actual, ok := getTeamID(response, request)
	if !ok || actual != teamID {
		t.Fatalf("expected team %s, got %s", teamID, actual)
	}
}
