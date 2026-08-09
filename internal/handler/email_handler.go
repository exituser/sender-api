package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sender-api/sender-api/internal/auth"
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
	r.Get("/dead-letters", h.ListDeadLetters)
	r.Post("/dead-letters/{id}/replay", h.ReplayDeadLetter)
	r.Get("/", h.List)
	r.Get("/{id}", h.GetByID)
	r.Get("/{id}/events", h.GetEvents)
	r.Post("/{id}/reconcile", h.ReconcileAmbiguous)
	r.Delete("/{id}", h.Cancel)
	return r
}

func (h *EmailHandler) ListDeadLetters(w http.ResponseWriter, r *http.Request) {
	if !requireRoles(w, r, "owner", "admin") {
		return
	}
	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}
	limit := 50
	if value := r.URL.Query().Get("limit"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			limit = parsed
		}
	}
	deadLetters, err := h.emailService.ListDeadLetters(r.Context(), teamID, limit)
	if err != nil {
		writeError(w, "failed to list dead letters", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, map[string]any{"data": deadLetters}, http.StatusOK)
}

func (h *EmailHandler) ReplayDeadLetter(w http.ResponseWriter, r *http.Request) {
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
	if err := h.emailService.ReplayDeadLetter(r.Context(), teamID, id); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]string{"status": "requeued", "id": id.String()}, http.StatusAccepted)
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

	resp, created, err := h.emailService.SendWithIdempotency(r.Context(), teamID, &req, r.Header.Get("Idempotency-Key"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrQueueUnavailable) {
			status = http.StatusServiceUnavailable
		}
		if errors.Is(err, service.ErrUsageUnavailable) {
			status = http.StatusServiceUnavailable
		}
		if errors.Is(err, service.ErrDailyRecipientLimit) {
			status = http.StatusTooManyRequests
		}
		if errors.Is(err, service.ErrIdempotencyConflict) {
			status = http.StatusConflict
		}
		writeError(w, err.Error(), status)
		return
	}

	if created {
		writeJSON(w, resp, http.StatusCreated)
		return
	}
	writeJSON(w, resp, http.StatusOK)
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
	batchKey := r.Header.Get("Idempotency-Key")
	if batchKey == "" {
		writeError(w, service.ErrBatchIdempotencyKeyRequired.Error(), http.StatusBadRequest)
		return
	}

	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	responses, err := h.emailService.BatchSend(r.Context(), teamID, reqs, batchKey)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrQueueUnavailable) {
			status = http.StatusServiceUnavailable
		}
		if errors.Is(err, service.ErrUsageUnavailable) {
			status = http.StatusServiceUnavailable
		}
		if errors.Is(err, service.ErrDailyRecipientLimit) {
			status = http.StatusTooManyRequests
		}
		if errors.Is(err, service.ErrIdempotencyConflict) {
			status = http.StatusConflict
		}
		if len(responses) > 0 {
			failedItem := len(responses) + 1
			var batchErr *service.BatchSendError
			if errors.As(err, &batchErr) {
				failedItem = batchErr.Index + 1
			}
			writeJSON(w, map[string]any{
				"data":        responses,
				"error":       "one message could not be queued; later messages were not processed",
				"failed_item": failedItem,
			}, http.StatusMultiStatus)
			return
		}
		writeError(w, err.Error(), status)
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
	claims, teamID, ok := getTeamID(w, r)
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
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, "email not found", http.StatusNotFound)
		} else {
			writeError(w, "failed to load email", http.StatusInternalServerError)
		}
		return
	}
	email.CanReconcile = auth.HasAnyRole(claims, "owner", "admin")

	writeJSON(w, email, http.StatusOK)
}

func (h *EmailHandler) ReconcileAmbiguous(w http.ResponseWriter, r *http.Request) {
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
	var req domain.ReconcileEmailRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	email, err := h.emailService.ReconcileAmbiguous(r.Context(), teamID, id, req)
	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			writeError(w, "email not found", http.StatusNotFound)
		case errors.Is(err, service.ErrInvalidDeliveryReviewAction):
			writeError(w, "choose accepted or failed", http.StatusBadRequest)
		case errors.Is(err, domain.ErrDeliveryConfirmationUnavailable):
			writeError(w, "delivery confirmation is not available yet", http.StatusConflict)
		case errors.Is(err, service.ErrDeliveryReviewNotNeeded):
			writeError(w, "this message no longer needs review", http.StatusConflict)
		default:
			writeError(w, "delivery review is temporarily unavailable", http.StatusServiceUnavailable)
		}
		return
	}
	email.CanReconcile = true
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
