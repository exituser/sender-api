package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
