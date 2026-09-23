package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/andrestorresgo/backend-service/internal/service"
)

// ActuatorCommander specifies the contract for handling remote actuation commands.
type ActuatorCommander interface {
	CommandServo(ctx context.Context, req service.ServoCommandRequest) (service.ServoCommandResult, error)
}

// ActuatorServoHandler handles POST /api/v1/actuator/servo requests.
func ActuatorServoHandler(commander ActuatorCommander) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if commander == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "actuator service unavailable",
			})
			return
		}

		var req service.ServoCommandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "invalid request payload",
				"message": err.Error(),
			})
			return
		}

		result, err := commander.CommandServo(r.Context(), req)
		if err != nil {
			if errors.Is(err, service.ErrInvalidServoPayload) {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": err.Error(),
				})
				return
			}
			if errors.Is(err, service.ErrActuatorPublisherUnavailable) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": err.Error(),
				})
				return
			}

			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "failed to dispatch servo command",
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	}
}
