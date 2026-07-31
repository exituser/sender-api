package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/internal/service"
)

type ContactHandler struct {
	contactService *service.ContactService
}

func NewContactHandler(contactService *service.ContactService) *ContactHandler {
	return &ContactHandler{contactService: contactService}
}

func (h *ContactHandler) Routes() http.Handler {
	r := http.NewServeMux()
	r.HandleFunc("POST /", h.Create)
	r.HandleFunc("GET /", h.List)
	r.HandleFunc("GET /{id}", h.GetByID)
	r.HandleFunc("PATCH /{id}", h.Update)
	r.HandleFunc("DELETE /{id}", h.Delete)
	r.HandleFunc("POST /import", h.ImportCSV)
	return r
}

func (h *ContactHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !requireRoles(w, r, "owner", "admin") {
		return
	}

	var req domain.CreateContactRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	contact, err := h.contactService.Create(r.Context(), teamID, &req)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, contact, http.StatusCreated)
}

func (h *ContactHandler) List(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := parseInt(l, 50); err == nil {
			limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := parseInt(o, 0); err == nil {
			offset = v
		}
	}

	result, err := h.contactService.List(r.Context(), teamID, limit, offset)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, result, http.StatusOK)
}

func (h *ContactHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}

	contact, err := h.contactService.GetByID(r.Context(), teamID, id)
	if err != nil {
		writeError(w, "contact not found", http.StatusNotFound)
		return
	}

	writeJSON(w, contact, http.StatusOK)
}

func (h *ContactHandler) Update(w http.ResponseWriter, r *http.Request) {
	if !requireRoles(w, r, "owner", "admin") {
		return
	}
	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}

	var req domain.UpdateContactRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	contact, err := h.contactService.Update(r.Context(), teamID, id, &req)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, contact, http.StatusOK)
}

func (h *ContactHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !requireRoles(w, r, "owner", "admin") {
		return
	}
	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.contactService.Delete(r.Context(), teamID, id); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ContactHandler) ImportCSV(w http.ResponseWriter, r *http.Request) {
	if !requireRoles(w, r, "owner", "admin") {
		return
	}

	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, "file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		writeError(w, "invalid csv format", http.StatusBadRequest)
		return
	}

	if len(records) < 2 {
		writeError(w, "csv must have header row and at least one data row", http.StatusBadRequest)
		return
	}

	header := records[0]
	colIndex := make(map[string]int)
	for i, h := range header {
		colIndex[strings.ToLower(strings.TrimSpace(h))] = i
	}

	emailIdx, hasEmail := colIndex["email"]
	if !hasEmail {
		writeError(w, "csv must have an 'email' column", http.StatusBadRequest)
		return
	}

	var contacts []*domain.CreateContactRequest
	for _, row := range records[1:] {
		if emailIdx >= len(row) || strings.TrimSpace(row[emailIdx]) == "" {
			continue
		}

		contact := &domain.CreateContactRequest{
			Email: strings.TrimSpace(row[emailIdx]),
		}

		if idx, ok := colIndex["first_name"]; ok && idx < len(row) {
			name := strings.TrimSpace(row[idx])
			if name != "" {
				contact.FirstName = &name
			}
		}
		if idx, ok := colIndex["last_name"]; ok && idx < len(row) {
			name := strings.TrimSpace(row[idx])
			if name != "" {
				contact.LastName = &name
			}
		}

		contacts = append(contacts, contact)
	}

	count, err := h.contactService.ImportCSV(r.Context(), teamID, contacts)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, map[string]int{"imported": count}, http.StatusCreated)
}

func parseJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func parseInt(s string, def int) (int, error) {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	if err != nil {
		return def, err
	}
	return v, nil
}
