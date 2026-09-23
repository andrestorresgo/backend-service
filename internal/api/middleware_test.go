package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andrestorresgo/backend-service/internal/api"
)

func TestBearerAuthMiddleware(t *testing.T) {
	expectedToken := "test-secret-bearer-token"
	mw := api.BearerAuthMiddleware(expectedToken)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"authorized"}`))
	})

	protected := mw(nextHandler)

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "Valid Bearer token",
			authHeader:     "Bearer test-secret-bearer-token",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Missing Authorization header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Incorrect token",
			authHeader:     "Bearer wrong-token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Malformed prefix Basic",
			authHeader:     "Basic test-secret-bearer-token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Missing Bearer prefix",
			authHeader:     "test-secret-bearer-token",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Bearer without token",
			authHeader:     "Bearer ",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rr := httptest.NewRecorder()

			protected.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

func TestBearerAuthMiddleware_EmptyConfigToken(t *testing.T) {
	mw := api.BearerAuthMiddleware("")

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	protected := mw(nextHandler)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Authorization", "Bearer ")
	rr := httptest.NewRecorder()

	protected.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 Unauthorized when configured token is empty, got %d", rr.Code)
	}
}
