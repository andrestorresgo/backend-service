package api

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"time"

	"github.com/andrestorresgo/backend-service/internal/db"
)

// DatabaseStatus describes the connectivity state of the PostgreSQL database.
type DatabaseStatus struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// MQTTStatus describes the connectivity state of the MQTT broker.
type MQTTStatus struct {
	Status string `json:"status"`
}

// HealthResponse represents the payload returned by GET /healthz.
type HealthResponse struct {
	Status   string         `json:"status"`
	Service  string         `json:"service"`
	Database DatabaseStatus `json:"database"`
	MQTT     MQTTStatus     `json:"mqtt"`
}

func isNil(i any) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.UnsafePointer, reflect.Interface, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// HealthHandler returns an HTTP handler for the /healthz liveness and readiness probe.
func HealthHandler(pinger db.DBPinger, broker BrokerStatusChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := "ok"
		dbStatus := DatabaseStatus{Status: "connected"}

		if isNil(pinger) {
			status = "degraded"
			dbStatus = DatabaseStatus{Status: "not_configured"}
		} else {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()

			if err := pinger.Ping(ctx); err != nil {
				status = "degraded"
				dbStatus = DatabaseStatus{
					Status: "disconnected",
					Error:  err.Error(),
				}
			}
		}

		mqttStatus := MQTTStatus{Status: "disconnected"}
		if !isNil(broker) && broker.IsConnected() {
			mqttStatus = MQTTStatus{Status: "connected"}
		} else if isNil(broker) {
			mqttStatus = MQTTStatus{Status: "not_configured"}
		}

		resp := HealthResponse{
			Status:   status,
			Service:  "backend-service",
			Database: dbStatus,
			MQTT:     mqttStatus,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}
