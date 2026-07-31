package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/auth"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/pkg/validator"
)

type WebhookRepository interface {
	Create(ctx context.Context, webhook *domain.Webhook) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Webhook, error)
	GetByIDForTeam(ctx context.Context, teamID, id uuid.UUID) (*domain.Webhook, error)
	List(ctx context.Context, teamID uuid.UUID) (*domain.WebhookListResponse, error)
	Update(ctx context.Context, webhook *domain.Webhook) error
	UpdateForTeam(ctx context.Context, webhook *domain.Webhook) error
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteForTeam(ctx context.Context, teamID, id uuid.UUID) error
}

type WebhookHandler struct {
	webhookRepo    WebhookRepository
	deliveryReader interface {
		ListForWebhook(context.Context, uuid.UUID, uuid.UUID, int) ([]domain.WebhookDelivery, error)
		CreateDelivery(context.Context, *domain.WebhookDelivery) error
	}
	requireHTTPS bool
}

func NewWebhookHandler(webhookRepo WebhookRepository, requireHTTPS ...bool) *WebhookHandler {
	production := len(requireHTTPS) > 0 && requireHTTPS[0]
	return &WebhookHandler{webhookRepo: webhookRepo, requireHTTPS: production}
}

func (h *WebhookHandler) SetDeliveryReader(reader interface {
	ListForWebhook(context.Context, uuid.UUID, uuid.UUID, int) ([]domain.WebhookDelivery, error)
	CreateDelivery(context.Context, *domain.WebhookDelivery) error
}) {
	h.deliveryReader = reader
}

func (h *WebhookHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.Create)
	r.Get("/", h.List)
	r.Get("/{id}/deliveries", h.ListDeliveries)
	r.Post("/{id}/test", h.Test)
	r.Get("/{id}", h.GetByID)
	r.Patch("/{id}", h.Update)
	r.Delete("/{id}", h.Delete)
	return r
}

func (h *WebhookHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !requireRoles(w, r, "owner", "admin") {
		return
	}

	var req domain.CreateWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	if !h.validWebhookURL(req.URL) || len(req.Events) == 0 || len(req.Events) > 50 {
		writeError(w, "valid url and at least one event are required", http.StatusBadRequest)
		return
	}
	for _, event := range req.Events {
		if event == "" || len(event) > 100 || strings.ContainsAny(event, "\r\n") {
			writeError(w, "invalid webhook event", http.StatusBadRequest)
			return
		}
	}

	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	secret, _, _, err := auth.GenerateAPIKeyInternal()
	if err != nil {
		writeError(w, "failed to generate webhook secret", http.StatusInternalServerError)
		return
	}

	webhook := &domain.Webhook{
		ID:     uuid.New(),
		TeamID: teamID,
		URL:    req.URL,
		Events: req.Events,
		Secret: secret,
		Active: true,
	}

	if err := h.webhookRepo.Create(r.Context(), webhook); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, domain.CreateWebhookResponse{
		ID:        webhook.ID,
		URL:       webhook.URL,
		Events:    webhook.Events,
		Active:    webhook.Active,
		Secret:    secret,
		CreatedAt: webhook.CreatedAt,
	}, http.StatusCreated)
}

func (h *WebhookHandler) List(w http.ResponseWriter, r *http.Request) {
	if !requirePermission(w, r, "read") {
		return
	}
	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	result, err := h.webhookRepo.List(r.Context(), teamID)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, result, http.StatusOK)
}

func (h *WebhookHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	if !requirePermission(w, r, "read") {
		return
	}
	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}

	webhook, err := h.webhookRepo.GetByIDForTeam(r.Context(), teamID, id)
	if err != nil {
		writeError(w, "webhook not found", http.StatusNotFound)
		return
	}

	writeJSON(w, webhook, http.StatusOK)
}

func (h *WebhookHandler) Update(w http.ResponseWriter, r *http.Request) {
	if !requireRoles(w, r, "owner", "admin") {
		return
	}
	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}

	webhook, err := h.webhookRepo.GetByIDForTeam(r.Context(), teamID, id)
	if err != nil {
		writeError(w, "webhook not found", http.StatusNotFound)
		return
	}

	var req domain.UpdateWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.URL != nil {
		trimmedURL := strings.TrimSpace(*req.URL)
		if !h.validWebhookURL(trimmedURL) {
			writeError(w, "invalid webhook url", http.StatusBadRequest)
			return
		}
		req.URL = &trimmedURL
	}
	if req.Events != nil {
		if len(*req.Events) == 0 || len(*req.Events) > 50 {
			writeError(w, "at least one webhook event is required", http.StatusBadRequest)
			return
		}
		for _, event := range *req.Events {
			if event == "" || len(event) > 100 || strings.ContainsAny(event, "\r\n") {
				writeError(w, "invalid webhook event", http.StatusBadRequest)
				return
			}
		}
	}

	if req.URL != nil {
		webhook.URL = *req.URL
	}
	if req.Events != nil {
		webhook.Events = *req.Events
	}
	if req.Active != nil {
		webhook.Active = *req.Active
	}

	if err := h.webhookRepo.UpdateForTeam(r.Context(), webhook); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, webhook, http.StatusOK)
}

func (h *WebhookHandler) validWebhookURL(rawURL string) bool {
	if !validator.IsValidURL(rawURL) {
		return false
	}
	if !h.requireHTTPS {
		return true
	}
	parsed, err := url.Parse(rawURL)
	return err == nil && strings.EqualFold(parsed.Scheme, "https")
}

func (h *WebhookHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !requireRoles(w, r, "owner", "admin") {
		return
	}
	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.webhookRepo.DeleteForTeam(r.Context(), teamID, id); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *WebhookHandler) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	if !requirePermission(w, r, "read") {
		return
	}
	if h.deliveryReader == nil {
		writeError(w, "webhook delivery history is not configured", http.StatusServiceUnavailable)
		return
	}
	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if _, err := h.webhookRepo.GetByIDForTeam(r.Context(), teamID, id); err != nil {
		writeError(w, "webhook not found", http.StatusNotFound)
		return
	}

	limit := 50
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsedLimit, parseErr := strconv.Atoi(rawLimit)
		if parseErr != nil || parsedLimit < 1 || parsedLimit > 100 {
			writeError(w, "limit must be between 1 and 100", http.StatusBadRequest)
			return
		}
		limit = parsedLimit
	}
	deliveries, err := h.deliveryReader.ListForWebhook(r.Context(), teamID, id, limit)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"data": deliveries}, http.StatusOK)
}

func (h *WebhookHandler) Test(w http.ResponseWriter, r *http.Request) {
	if !requireRoles(w, r, "owner", "admin") {
		return
	}
	if h.deliveryReader == nil {
		writeError(w, "webhook delivery queue is not configured", http.StatusServiceUnavailable)
		return
	}
	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}
	webhook, err := h.webhookRepo.GetByIDForTeam(r.Context(), teamID, id)
	if err != nil {
		writeError(w, "webhook not found", http.StatusNotFound)
		return
	}
	if !webhook.Active {
		writeError(w, "webhook is disabled", http.StatusConflict)
		return
	}

	eventID := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"id":         eventID,
		"type":       "webhook.test",
		"webhook_id": webhook.ID,
		"team_id":    teamID,
		"created_at": time.Now().UTC(),
		"data": map[string]any{
			"message": "This is a test webhook delivery.",
		},
	})
	if err != nil {
		writeError(w, "failed to create test payload", http.StatusInternalServerError)
		return
	}
	delivery := &domain.WebhookDelivery{
		ID:        uuid.New(),
		WebhookID: webhook.ID,
		EventID:   eventID,
		Event:     "webhook.test",
		Payload:   payload,
		Status:    domain.WebhookDeliveryPending,
	}
	if err := h.deliveryReader.CreateDelivery(r.Context(), delivery); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"id":       delivery.ID,
		"event_id": delivery.EventID,
		"status":   domain.WebhookDeliveryPending,
		"message":  "test delivery queued",
	}, http.StatusAccepted)
}
