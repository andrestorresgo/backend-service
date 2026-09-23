package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/andrestorresgo/backend-service/internal/service"
)

// Authenticator defines the domain contract for processing authentication requests.
type Authenticator interface {
	Authenticate(ctx context.Context, req service.AuthRequest) (service.AuthResponse, error)
}

// LoginRequest defines the expected JSON payload for POST /api/v1/auth/login.
type LoginRequest struct {
	UserID int    `json:"user_id"`
	PIN    string `json:"pin"`
}

// AuthLoginHandler handles dashboard authentication requests and maps outcomes to semantic HTTP statuses.
func AuthLoginHandler(auth Authenticator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if auth == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "authentication service is not available",
			})
			return
		}

		var req LoginRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "invalid request payload",
				"message": err.Error(),
			})
			return
		}

		if req.UserID <= 0 || req.PIN == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "user_id must be positive and pin must not be empty",
			})
			return
		}

		authReq := service.AuthRequest{
			UserID: req.UserID,
			PIN:    req.PIN,
			Source: service.AuthSourceDashboard,
		}

		authResp, err := auth.Authenticate(r.Context(), authReq)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "internal authentication error",
			})
			return
		}

		switch authResp.Status {
		case service.AuthStatusOK:
			w.WriteHeader(http.StatusOK)
		case service.AuthStatusInvalidPIN:
			w.WriteHeader(http.StatusUnauthorized)
		case service.AuthStatusUserLocked:
			w.WriteHeader(http.StatusForbidden)
		case service.AuthStatusUserNotFound:
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}

		_ = json.NewEncoder(w).Encode(authResp)
	}
}
