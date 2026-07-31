package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/sender-api/sender-api/internal/auth"
	"github.com/sender-api/sender-api/internal/domain"
	"github.com/sender-api/sender-api/internal/service"
)

type TeamHandler struct {
	teamService *service.TeamService
}

func (h *TeamHandler) authorizeTeam(w http.ResponseWriter, r *http.Request, teamID uuid.UUID, roles ...string) bool {
	claims, ok := getClaims(w, r)
	if !ok {
		return false
	}
	if claims.Role == "api_key" {
		writeError(w, "api keys cannot manage teams", http.StatusForbidden)
		return false
	}
	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		writeError(w, "invalid user id", http.StatusUnauthorized)
		return false
	}
	member, err := h.teamService.GetMember(r.Context(), teamID, userID)
	if err != nil {
		writeError(w, "forbidden", http.StatusForbidden)
		return false
	}
	if len(roles) > 0 && !auth.HasAnyRole(&auth.Claims{Role: string(member.Role)}, roles...) {
		writeError(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func NewTeamHandler(teamService *service.TeamService) *TeamHandler {
	return &TeamHandler{teamService: teamService}
}

func (h *TeamHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.Create)
	r.Get("/", h.List)
	r.Post("/invitations/accept", h.AcceptInvitation)
	r.Get("/{id}", h.GetByID)
	r.Patch("/{id}", h.Update)
	r.Delete("/{id}", h.Delete)
	r.Post("/{id}/invite", h.InviteMember)
	r.Get("/{id}/invitations", h.ListInvitations)
	r.Delete("/{id}/invitations/{invitationId}", h.RevokeInvitation)
	r.Delete("/{id}/members/{userId}", h.RemoveMember)
	r.Patch("/{id}/members/{userId}/role", h.UpdateMemberRole)
	r.Get("/{id}/members", h.GetMembers)
	return r
}

func (h *TeamHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims, ok := getClaims(w, r)
	if !ok {
		return
	}
	if claims.Role == "api_key" {
		writeError(w, "api keys cannot create teams", http.StatusForbidden)
		return
	}

	var req domain.CreateTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		writeError(w, "invalid user id", http.StatusBadRequest)
		return
	}

	team, err := h.teamService.Create(r.Context(), userID, &req)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, team, http.StatusCreated)
}

func (h *TeamHandler) List(w http.ResponseWriter, r *http.Request) {
	claims, ok := getClaims(w, r)
	if !ok {
		return
	}
	if claims.Role == "api_key" {
		writeError(w, "api keys cannot list user teams", http.StatusForbidden)
		return
	}

	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		writeError(w, "invalid user id", http.StatusBadRequest)
		return
	}

	teams, err := h.teamService.ListByUser(r.Context(), userID)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, teams, http.StatusOK)
}

func (h *TeamHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if !h.authorizeTeam(w, r, id) {
		return
	}

	team, err := h.teamService.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, "team not found", http.StatusNotFound)
		return
	}

	writeJSON(w, team, http.StatusOK)
}

func (h *TeamHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if !h.authorizeTeam(w, r, id, "owner", "admin") {
		return
	}

	var req domain.UpdateTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	team, err := h.teamService.Update(r.Context(), id, &req)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, team, http.StatusOK)
}

func (h *TeamHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if !h.authorizeTeam(w, r, id, "owner") {
		return
	}

	if err := h.teamService.Delete(r.Context(), id); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TeamHandler) InviteMember(w http.ResponseWriter, r *http.Request) {
	teamID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, "invalid team id", http.StatusBadRequest)
		return
	}
	if !h.authorizeTeam(w, r, teamID, "owner", "admin") {
		return
	}

	var req domain.InviteMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	invitation, err := h.teamService.CreateInvitation(r.Context(), teamID, &req)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, invitation, http.StatusCreated)
}

func (h *TeamHandler) ListInvitations(w http.ResponseWriter, r *http.Request) {
	teamID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, "invalid team id", http.StatusBadRequest)
		return
	}
	if !h.authorizeTeam(w, r, teamID, "owner", "admin") {
		return
	}
	invitations, err := h.teamService.ListInvitations(r.Context(), teamID)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, invitations, http.StatusOK)
}

func (h *TeamHandler) RevokeInvitation(w http.ResponseWriter, r *http.Request) {
	teamID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, "invalid team id", http.StatusBadRequest)
		return
	}
	if !h.authorizeTeam(w, r, teamID, "owner", "admin") {
		return
	}
	invitationID, err := uuid.Parse(chi.URLParam(r, "invitationId"))
	if err != nil {
		writeError(w, "invalid invitation id", http.StatusBadRequest)
		return
	}
	if err := h.teamService.RevokeInvitation(r.Context(), teamID, invitationID); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *TeamHandler) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	claims, ok := getClaims(w, r)
	if !ok {
		return
	}
	if claims.Role == "api_key" {
		writeError(w, "api keys cannot accept invitations", http.StatusForbidden)
		return
	}
	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		writeError(w, "invalid user id", http.StatusUnauthorized)
		return
	}
	var req domain.AcceptInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	invitation, err := h.teamService.AcceptInvitation(r.Context(), req.Token, userID)
	if err != nil {
		writeError(w, "invitation is invalid or expired", http.StatusBadRequest)
		return
	}
	writeJSON(w, invitation, http.StatusOK)
}

func (h *TeamHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	teamID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, "invalid team id", http.StatusBadRequest)
		return
	}
	if !h.authorizeTeam(w, r, teamID, "owner", "admin") {
		return
	}

	userID, err := uuid.Parse(chi.URLParam(r, "userId"))
	if err != nil {
		writeError(w, "invalid user id", http.StatusBadRequest)
		return
	}

	if err := h.teamService.RemoveMember(r.Context(), teamID, userID); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TeamHandler) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	teamID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, "invalid team id", http.StatusBadRequest)
		return
	}
	if !h.authorizeTeam(w, r, teamID, "owner", "admin") {
		return
	}

	userID, err := uuid.Parse(chi.URLParam(r, "userId"))
	if err != nil {
		writeError(w, "invalid user id", http.StatusBadRequest)
		return
	}

	var req struct {
		Role domain.TeamMemberRole `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.teamService.UpdateMemberRole(r.Context(), teamID, userID, req.Role); err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *TeamHandler) GetMembers(w http.ResponseWriter, r *http.Request) {
	teamID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, "invalid team id", http.StatusBadRequest)
		return
	}
	if !h.authorizeTeam(w, r, teamID) {
		return
	}

	members, err := h.teamService.GetMembers(r.Context(), teamID)
	if err != nil {
		writeError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, members, http.StatusOK)
}
