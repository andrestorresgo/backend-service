package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/andrestorresgo/backend-service/internal/service"
)

// BrokerStatusChecker abstracts checking if the MQTT broker is currently connected.
type BrokerStatusChecker interface {
	IsConnected() bool
}

// StateSnapshotProvider specifies the domain contract for aggregating initial telemetry snapshot and recent actions.
type StateSnapshotProvider interface {
	GetSnapshot(ctx context.Context, mqttConnected bool) (service.StateSnapshot, error)
	GetRecentActions(ctx context.Context, limit int) ([]service.ActionRecord, error)
}

// StateHandler handles GET /api/v1/state delivering a consolidated system snapshot.
func StateHandler(provider StateSnapshotProvider, broker BrokerStatusChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if provider == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "state service unavailable",
			})
			return
		}

		mqttConnected := false
		if broker != nil && broker.IsConnected() {
			mqttConnected = true
		}

		snapshot, err := provider.GetSnapshot(r.Context(), mqttConnected)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "failed to retrieve system state snapshot",
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(snapshot)
	}
}
