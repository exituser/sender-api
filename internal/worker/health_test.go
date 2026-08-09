package worker

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthHandlerRequiresReadyFreshState(t *testing.T) {
	state := NewHealthState()
	handler := NewHealthHandler(state)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	state.SetReady(true)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !state.Healthy(time.Now(), 2*time.Minute) {
		t.Fatal("ready state should be healthy")
	}
}
