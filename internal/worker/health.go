package worker

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"
)

type HealthState struct {
	ready         atomic.Bool
	lastHeartbeat atomic.Int64
}

func NewHealthState() *HealthState         { h := &HealthState{}; h.Heartbeat(); return h }
func (h *HealthState) SetReady(ready bool) { h.ready.Store(ready); h.Heartbeat() }
func (h *HealthState) Heartbeat()          { h.lastHeartbeat.Store(time.Now().UnixNano()) }
func (h *HealthState) Healthy(now time.Time, maxAge time.Duration) bool {
	return h.ready.Load() && now.Sub(time.Unix(0, h.lastHeartbeat.Load())) <= maxAge
}

type HealthHandler struct {
	states []*HealthState
	checks []func(context.Context) error
	maxAge time.Duration
}

func NewHealthHandler(states ...*HealthState) *HealthHandler {
	return &HealthHandler{states: states, maxAge: 2 * time.Minute}
}
func (h *HealthHandler) AddChecks(checks ...func(context.Context) error) {
	h.checks = append(h.checks, checks...)
}
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ready := len(h.states) > 0
	for _, state := range h.states {
		if state == nil || !state.Healthy(time.Now(), h.maxAge) {
			ready = false
			break
		}
	}
	if ready {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		for _, check := range h.checks {
			if check != nil && check(ctx) != nil {
				ready = false
				break
			}
		}
	}
	if !ready {
		http.Error(w, "not ready\n", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}
func (h *HealthHandler) ReadinessHandler() http.Handler { return h }
