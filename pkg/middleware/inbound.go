package middleware

import (
	"crypto/subtle"
	"net/http"
)

func InboundToken(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				writeInboundError(w, "inbound webhook token is not configured", http.StatusServiceUnavailable)
				return
			}

			provided := r.Header.Get("X-Inbound-Token")
			if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
				writeInboundError(w, "invalid inbound webhook token", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func writeInboundError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + message + `"}`))
}
