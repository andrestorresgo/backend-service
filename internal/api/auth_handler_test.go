package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andrestorresgo/backend-service/internal/api"
	"github.com/andrestorresgo/backend-service/internal/config"
	"github.com/andrestorresgo/backend-service/internal/service"
)

type mockAuthenticator struct {
	authenticateFn func(ctx context.Context, req service.AuthRequest) (service.AuthResponse, error)
}

func (m *mockAuthenticator) Authenticate(ctx context.Context, req service.AuthRequest) (service.AuthResponse, error) {
	if m.authenticateFn != nil {
		return m.authenticateFn(ctx, req)
	}
	return service.AuthResponse{}, errors.New("not implemented")
}

func setupAuthTestRouter(auth api.Authenticator) *httptest.Server {
	cfg := &config.Config{
		CORSAllowedOrigins: []string{"*"},
	}
	router := api.NewRouter(cfg, nil, auth, nil)
	return httptest.NewServer(router)
}

func TestAuthLoginHandler_SemanticStatusCodes(t *testing.T) {
	tests := []struct {
		name                 string
		payload              string
		authResp             service.AuthResponse
		authErr              error
		expectedStatus       int
		expectedStatusField  string
		expectedUsername     string
		expectedRemaining    int
		expectedLockoutSec   int
		expectedSourcePassed service.AuthSource
	}{
		{
			name:    "200 OK on successful authentication",
			payload: `{"user_id": 1, "pin": "1234"}`,
			authResp: service.AuthResponse{
				Status:            service.AuthStatusOK,
				Username:          "Andres",
				RemainingAttempts: 2,
				LockoutSeconds:    0,
			},
			expectedStatus:       http.StatusOK,
			expectedStatusField:  "AUTH_OK",
			expectedUsername:     "Andres",
			expectedRemaining:    2,
			expectedLockoutSec:   0,
			expectedSourcePassed: service.AuthSourceDashboard,
		},
		{
			name:    "401 Unauthorized on invalid PIN",
			payload: `{"user_id": 1, "pin": "9999"}`,
			authResp: service.AuthResponse{
				Status:            service.AuthStatusInvalidPIN,
				RemainingAttempts: 1,
				LockoutSeconds:    0,
			},
			expectedStatus:       http.StatusUnauthorized,
			expectedStatusField:  "INVALID_PIN",
			expectedRemaining:    1,
			expectedLockoutSec:   0,
			expectedSourcePassed: service.AuthSourceDashboard,
		},
		{
			name:    "403 Forbidden when user is locked",
			payload: `{"user_id": 1, "pin": "1234"}`,
			authResp: service.AuthResponse{
				Status:            service.AuthStatusUserLocked,
				RemainingAttempts: 0,
				LockoutSeconds:    45,
			},
			expectedStatus:       http.StatusForbidden,
			expectedStatusField:  "USER_LOCKED",
			expectedRemaining:    0,
			expectedLockoutSec:   45,
			expectedSourcePassed: service.AuthSourceDashboard,
		},
		{
			name:    "404 Not Found when user does not exist",
			payload: `{"user_id": 999, "pin": "1234"}`,
			authResp: service.AuthResponse{
				Status:            service.AuthStatusUserNotFound,
				RemainingAttempts: 0,
				LockoutSeconds:    0,
			},
			expectedStatus:       http.StatusNotFound,
			expectedStatusField:  "USER_NOT_FOUND",
			expectedRemaining:    0,
			expectedLockoutSec:   0,
			expectedSourcePassed: service.AuthSourceDashboard,
		},
		{
			name:           "400 Bad Request on invalid JSON",
			payload:        `{invalid json`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "400 Bad Request on empty PIN",
			payload:        `{"user_id": 1, "pin": ""}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "400 Bad Request on invalid user_id",
			payload:        `{"user_id": 0, "pin": "1234"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "500 Internal Server Error when auth service fails",
			payload:        `{"user_id": 1, "pin": "1234"}`,
			authErr:        errors.New("db connection failure"),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedSource service.AuthSource

			mockAuth := &mockAuthenticator{
				authenticateFn: func(ctx context.Context, req service.AuthRequest) (service.AuthResponse, error) {
					capturedSource = req.Source
					if tt.authErr != nil {
						return service.AuthResponse{}, tt.authErr
					}
					return tt.authResp, nil
				},
			}

			server := setupAuthTestRouter(mockAuth)
			defer server.Close()

			res, err := http.Post(server.URL+"/api/v1/auth/login", "application/json", bytes.NewBufferString(tt.payload))
			if err != nil {
				t.Fatalf("unexpected POST error: %v", err)
			}
			defer res.Body.Close()

			if res.StatusCode != tt.expectedStatus {
				t.Errorf("status code got %d, want %d", res.StatusCode, tt.expectedStatus)
			}

			if tt.expectedStatusField != "" {
				var resp service.AuthResponse
				if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response JSON: %v", err)
				}

				if string(resp.Status) != tt.expectedStatusField {
					t.Errorf("status field got %s, want %s", resp.Status, tt.expectedStatusField)
				}
				if resp.Username != tt.expectedUsername {
					t.Errorf("username got %s, want %s", resp.Username, tt.expectedUsername)
				}
				if resp.RemainingAttempts != tt.expectedRemaining {
					t.Errorf("remaining attempts got %d, want %d", resp.RemainingAttempts, tt.expectedRemaining)
				}
				if resp.LockoutSeconds != tt.expectedLockoutSec {
					t.Errorf("lockout seconds got %d, want %d", resp.LockoutSeconds, tt.expectedLockoutSec)
				}

				if capturedSource != tt.expectedSourcePassed {
					t.Errorf("captured source got %s, want %s", capturedSource, tt.expectedSourcePassed)
				}
			}
		})
	}
}
