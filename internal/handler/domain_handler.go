package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/internal/service"
)

type DomainHandler struct {
	domainService *service.DomainService
}

func NewDomainHandler(domainService *service.DomainService) *DomainHandler {
	return &DomainHandler{domainService: domainService}
}

func (h *DomainHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.Create)
	r.Get("/", h.List)
	r.Get("/{id}", h.GetByID)
	r.Post("/{id}/verify", h.Verify)
	r.Delete("/{id}", h.Delete)
	return r
}

func (h *DomainHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !requireRoles(w, r, "owner", "admin") {
		return
	}

	var req domain.CreateDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	domainResp, err := h.domainService.Create(r.Context(), teamID, &req)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, domainResp, http.StatusCreated)
}

func (h *DomainHandler) List(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	result, err := h.domainService.List(r.Context(), teamID)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, result, http.StatusOK)
}

func (h *DomainHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}

	domainResp, err := h.domainService.GetByID(r.Context(), teamID, id)
	if err != nil {
		writeError(w, "domain not found", http.StatusNotFound)
		return
	}

	writeJSON(w, domainResp, http.StatusOK)
}

func (h *DomainHandler) Verify(w http.ResponseWriter, r *http.Request) {
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

	if err := h.domainService.Verify(r.Context(), teamID, id); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *DomainHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

	if err := h.domainService.Delete(r.Context(), teamID, id); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
