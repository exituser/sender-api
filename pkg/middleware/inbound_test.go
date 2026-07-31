package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInboundTokenRequiresConfiguredSecret(t *testing.T) {
	handler := InboundToken("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	cases := map[string]struct {
		token  string
		status int
	}{
		"missing": {token: "", status: http.StatusUnauthorized},
		"wrong":   {token: "wrong", status: http.StatusUnauthorized},
		"valid":   {token: "secret", status: http.StatusNoContent},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/inbound/ses", nil)
			if testCase.token != "" {
				request.Header.Set("X-Inbound-Token", testCase.token)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != testCase.status {
				t.Fatalf("expected %d, got %d", testCase.status, response.Code)
			}
		})
	}
}
