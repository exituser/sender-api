package handler

import (
	"net/http"

	"github.com/sender-api/sender-api/internal/service"
)

type DashboardHandler struct {
	dashboardService *service.DashboardService
}

func NewDashboardHandler(dashboardService *service.DashboardService) *DashboardHandler {
	return &DashboardHandler{dashboardService: dashboardService}
}

func (h *DashboardHandler) Summary(w http.ResponseWriter, r *http.Request) {
	if !requirePermission(w, r, "read") {
		return
	}
	_, teamID, ok := getTeamID(w, r)
	if !ok {
		return
	}

	summary, err := h.dashboardService.Summary(r.Context(), teamID)
	if err != nil {
		writeError(w, "Unable to load dashboard. Try again in a moment.", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, summary, http.StatusOK)
}
