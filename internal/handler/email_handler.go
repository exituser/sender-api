package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/internal/service"
)

type EmailHandler struct {
	emailService *service.EmailService
}

func NewEmailHandler(emailService *service.EmailService) *EmailHandler {
	return &EmailHandler{emailService: emailService}
}

func (h *EmailHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.Send)
	r.Post("/batch", h.BatchSend)
	r.Get("/", h.List)
	r.Get("/{id}", h.GetByID)
	r.Get("/{id}/events", h.GetEvents)
	r.Delete("/{id}", h.Cancel)
	return r
}

func (h *EmailHandler) Send(w http.ResponseWriter, r *http.Request) {
	if !requirePermission(w, r, "send") {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	var req domain.SendEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	resp, err := h.emailService.Send(r.Context(), teamID, &req)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrQueueUnavailable) {
			status = http.StatusServiceUnavailable
		}
		writeError(w, err.Error(), status)
		return
	}

	writeJSON(w, resp, http.StatusCreated)
}

func (h *EmailHandler) BatchSend(w http.ResponseWriter, r *http.Request) {
	if !requirePermission(w, r, "send") {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	var reqs []*domain.SendEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(reqs) == 0 {
		writeError(w, "at least one email is required", http.StatusBadRequest)
		return
	}

	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	responses, err := h.emailService.BatchSend(r.Context(), teamID, reqs)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, responses, http.StatusCreated)
}

func (h *EmailHandler) List(w http.ResponseWriter, r *http.Request) {
	if !requirePermission(w, r, "read") {
		return
	}

	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil {
			offset = v
		}
	}

	result, err := h.emailService.List(r.Context(), teamID, limit, offset)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, result, http.StatusOK)
}

func (h *EmailHandler) GetByID(w http.ResponseWriter, r *http.Request) {
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

	email, err := h.emailService.GetByID(r.Context(), teamID, id)
	if err != nil {
		writeError(w, "email not found", http.StatusNotFound)
		return
	}

	writeJSON(w, email, http.StatusOK)
}

func (h *EmailHandler) GetEvents(w http.ResponseWriter, r *http.Request) {
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

	events, err := h.emailService.GetEvents(r.Context(), teamID, id)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, events, http.StatusOK)
}

func (h *EmailHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	if !requirePermission(w, r, "send") {
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

	if err := h.emailService.Cancel(r.Context(), teamID, id); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
