package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

var httpRequests atomic.Uint64
var httpServerErrors atomic.Uint64

func HTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writer := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(writer, r)
		httpRequests.Add(1)
		if writer.status >= http.StatusInternalServerError {
			httpServerErrors.Add(1)
		}
	})
}

func Handler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "sender_api_http_requests_total %d\n", httpRequests.Load())
	fmt.Fprintf(w, "sender_api_http_server_errors_total %d\n", httpServerErrors.Load())
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}
