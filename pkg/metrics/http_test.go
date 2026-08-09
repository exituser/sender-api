package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPMetricsCountsRequestsAndServerErrors(t *testing.T) {
	beforeRequests := httpRequests.Load()
	beforeErrors := httpServerErrors.Load()
	handler := HTTP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if httpRequests.Load() != beforeRequests+1 || httpServerErrors.Load() != beforeErrors+1 {
		t.Fatal("expected request and error counters to increase")
	}
}

func TestWorkerMetricsExposeCountersAndAgeWithoutLabels(t *testing.T) {
	worker := NewWorkerMetrics("test_worker")
	worker.Start()
	worker.ObserveAge(1500 * time.Millisecond)
	worker.Complete()
	if snapshot := worker.Snapshot(); snapshot.Started != 1 || snapshot.Completed != 1 || snapshot.InFlight != 0 || snapshot.OldestAge != 1500*time.Millisecond {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}

	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := recorder.Body.String()
	if !strings.Contains(body, "sender_api_worker_test_worker_completed_total 1") {
		t.Fatal("worker completion metric missing")
	}
	if strings.Contains(body, "payload") || strings.Contains(body, "email") || strings.Contains(body, "message") {
		t.Fatal("worker metrics must not expose payload labels")
	}
}

func TestCustomMetricsAreValidatedAndRendered(t *testing.T) {
	AddCounter("sender_api_test_events_total", 2)
	SetGauge("sender_api_test_depth", 7)
	AddCounter("invalid metric label", 9)

	recorder := httptest.NewRecorder()
	Handler(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := recorder.Body.String()
	if !strings.Contains(body, "sender_api_test_events_total 2") || !strings.Contains(body, "sender_api_test_depth 7") {
		t.Fatalf("custom metrics missing from output: %s", body)
	}
	if strings.Contains(body, "invalid metric label") {
		t.Fatal("invalid metric name must not be rendered")
	}
}
