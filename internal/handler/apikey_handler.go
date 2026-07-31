package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/auth"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/pkg/validator"
)

type APIKeyRepository interface {
	Create(ctx context.Context, key *domain.APIKey) error
	List(ctx context.Context, teamID uuid.UUID) ([]domain.APIKey, error)
	DeleteForTeam(ctx context.Context, teamID, id uuid.UUID) error
}

type APIKeyHandler struct {
	apiKeyRepo APIKeyRepository
}

func NewAPIKeyHandler(apiKeyRepo APIKeyRepository) *APIKeyHandler {
	return &APIKeyHandler{apiKeyRepo: apiKeyRepo}
}

func (h *APIKeyHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.Create)
	r.Get("/", h.List)
	r.Delete("/{id}", h.Delete)
	return r
}

func (h *APIKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !requireRoles(w, r, "owner", "admin") {
		return
	}

	var req domain.CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 255 {
		writeError(w, "api key name must be between 1 and 255 characters", http.StatusBadRequest)
		return
	}

	if len(req.Permissions) == 0 {
		req.Permissions = []string{"send"}
	}
	for _, permission := range req.Permissions {
		if !validator.ContainsString([]string{"send", "read", "*"}, permission) {
			writeError(w, "unsupported api key permission", http.StatusBadRequest)
			return
		}
	}

	rawKey, hash, prefix, err := auth.GenerateAPIKeyInternal()
	if err != nil {
		writeError(w, "failed to generate key", http.StatusInternalServerError)
		return
	}

	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	key := &domain.APIKey{
		ID:          uuid.New(),
		TeamID:      teamID,
		Name:        req.Name,
		KeyHash:     hash,
		KeyPrefix:   prefix,
		Permissions: req.Permissions,
	}

	if err := h.apiKeyRepo.Create(r.Context(), key); err != nil {
		writeError(w, "failed to save key", http.StatusInternalServerError)
		return
	}

	writeJSON(w, domain.CreateAPIKeyResponse{
		ID:        key.ID,
		Name:      key.Name,
		Key:       rawKey,
		KeyPrefix: prefix,
	}, http.StatusCreated)
}

func (h *APIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	if !requireRoles(w, r, "owner", "admin") {
		return
	}

	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	keys, err := h.apiKeyRepo.List(r.Context(), teamID)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, keys, http.StatusOK)
}

func (h *APIKeyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !requireRoles(w, r, "owner", "admin") {
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}

	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	if err := h.apiKeyRepo.DeleteForTeam(r.Context(), teamID, id); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
