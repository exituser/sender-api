package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sender-api/sender-api/internal/service"
)

type UnsubscribeHandler struct {
	service *service.UnsubscribeService
}

func NewUnsubscribeHandler(unsubscribeService *service.UnsubscribeService) *UnsubscribeHandler {
	return &UnsubscribeHandler{service: unsubscribeService}
}

// Get intentionally does not mutate state. Mail clients, security scanners,
// and link prefetchers commonly issue GET requests against message links.
func (h *UnsubscribeHandler) Get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Unsubscribe</title></head><body><main style="font-family:system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem"><h1>Unsubscribe</h1><p>Use the button below to stop future emails from this sender.</p><form method="post"><button type="submit">Unsubscribe</button></form></main></body></html>`))
}

func (h *UnsubscribeHandler) Post(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if err := h.service.Unsubscribe(r.Context(), chi.URLParam(r, "token")); err != nil {
		if errors.Is(err, service.ErrInvalidUnsubscribeToken) {
			writeError(w, "invalid unsubscribe link", http.StatusBadRequest)
			return
		}
		writeError(w, "unsubscribe is temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
