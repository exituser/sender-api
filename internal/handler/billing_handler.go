package handler

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/internal/service"
)

type BillingHandler struct {
	billingService *service.BillingService
}

func NewBillingHandler(billingService *service.BillingService) *BillingHandler {
	return &BillingHandler{billingService: billingService}
}

func (h *BillingHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.Summary)
	r.Post("/checkout", h.Checkout)
	r.Post("/portal", h.Portal)
	return r
}

func (h *BillingHandler) Summary(w http.ResponseWriter, r *http.Request) {
	if !requirePermission(w, r, "read") {
		return
	}
	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}
	summary, err := h.billingService.Summary(r.Context(), teamID)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, summary, http.StatusOK)
}

func (h *BillingHandler) Checkout(w http.ResponseWriter, r *http.Request) {
	if !requireRoles(w, r, "owner", "admin") {
		return
	}
	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}
	var req domain.BillingCheckoutRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	session, err := h.billingService.Checkout(r.Context(), teamID, req.Plan)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrBillingNotConfigured) {
			status = http.StatusServiceUnavailable
		}
		writeError(w, err.Error(), status)
		return
	}
	writeJSON(w, session, http.StatusOK)
}

func (h *BillingHandler) Portal(w http.ResponseWriter, r *http.Request) {
	if !requireRoles(w, r, "owner", "admin") {
		return
	}
	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}
	session, err := h.billingService.Portal(r.Context(), teamID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrBillingNotConfigured) {
			status = http.StatusServiceUnavailable
		}
		writeError(w, err.Error(), status)
		return
	}
	writeJSON(w, session, http.StatusOK)
}

func (h *BillingHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, "invalid webhook body", http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()
	if err := h.billingService.HandleWebhook(r.Context(), payload, r.Header.Get("Stripe-Signature"), time.Now().UTC()); err != nil {
		if errors.Is(err, service.ErrBillingNotConfigured) {
			writeError(w, "billing webhook is not configured", http.StatusServiceUnavailable)
			return
		}
		writeError(w, "invalid billing webhook", http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]string{"status": "processed"}, http.StatusOK)
}
