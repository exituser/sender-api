package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/auth"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, msg string, code int) {
	if code >= http.StatusInternalServerError || isInternalErrorMessage(msg) {
		msg = "internal server error"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: msg})
}

func isInternalErrorMessage(msg string) bool {
	lower := strings.ToLower(msg)
	for _, marker := range []string{
		"failed to create ",
		"failed to save ",
		"failed to update ",
		"failed to delete ",
		"database",
		"duplicate key",
		"pgx",
		"pq:",
		"sql:",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, data any, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func getClaims(w http.ResponseWriter, r *http.Request) (*auth.Claims, bool) {
	claims := auth.GetClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	return claims, true
}

func getTeamID(w http.ResponseWriter, r *http.Request) (*auth.Claims, uuid.UUID, bool) {
	claims, ok := getClaims(w, r)
	if !ok {
		return nil, uuid.Nil, false
	}
	if claims.TeamID == "" {
		writeError(w, "team context is required", http.StatusBadRequest)
		return nil, uuid.Nil, false
	}
	teamID, err := uuid.Parse(claims.TeamID)
	if err != nil {
		writeError(w, "invalid team id", http.StatusBadRequest)
		return nil, uuid.Nil, false
	}
	return claims, teamID, true
}

func requireRoles(w http.ResponseWriter, r *http.Request, roles ...string) bool {
	claims, ok := getClaims(w, r)
	if !ok {
		return false
	}
	if !auth.HasAnyRole(claims, roles...) {
		writeError(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func requirePermission(w http.ResponseWriter, r *http.Request, permission string) bool {
	claims, ok := getClaims(w, r)
	if !ok {
		return false
	}
	if !auth.HasPermission(claims, permission) {
		writeError(w, "api key permission denied", http.StatusForbidden)
		return false
	}
	return true
}
