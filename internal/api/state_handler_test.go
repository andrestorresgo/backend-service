package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/andrestorresgo/backend-service/internal/api"
	"github.com/andrestorresgo/backend-service/internal/service"
)

type mockBrokerChecker struct {
	connected bool
}

func (m *mockBrokerChecker) IsConnected() bool {
	return m.connected
}

type mockStateSnapshotProvider struct {
	getSnapshotFn func(ctx context.Context, mqttConnected bool) (service.StateSnapshot, error)
}

func (m *mockStateSnapshotProvider) GetSnapshot(ctx context.Context, mqttConnected bool) (service.StateSnapshot, error) {
	if m.getSnapshotFn != nil {
		return m.getSnapshotFn(ctx, mqttConnected)
	}
	return service.StateSnapshot{}, errors.New("not implemented")
}

func TestStateHandler(t *testing.T) {
	now := time.Now()
	uid := 1
	sampleSnapshot := service.StateSnapshot{
		SystemState: &service.SystemState{
			ID:              1,
			IsPaused:        false,
			MotorState:      true,
			ServoState:      false,
			LastTelemetryAt: &now,
		},
		ShapeCounts: []service.ShapeCount{
			{ShapeID: 1, ShapeName: "circle", ColorLabel: "red", LiveBuffer: 2, TotalLifetime: 10},
			{ShapeID: 2, ShapeName: "triangle", ColorLabel: "green", LiveBuffer: 1, TotalLifetime: 5},
			{ShapeID: 3, ShapeName: "square", ColorLabel: "blue", LiveBuffer: 0, TotalLifetime: 15},
		},
		RecentAudits: []service.AuditRecord{
			{
				ID:        "audit-uuid-1",
				Source:    service.AuthSourceDashboard,
				UserID:    &uid,
				Status:    service.AuditStatusSuccess,
				Timestamp: now,
			},
		},
		MQTTConnected: true,
	}

	tests := []struct {
		name                 string
		provider             api.StateSnapshotProvider
		broker               api.BrokerStatusChecker
		expectedStatus       int
		expectedConnected    bool
		expectErrorSubstring string
	}{
		{
			name: "200 OK with connected broker",
			provider: &mockStateSnapshotProvider{
				getSnapshotFn: func(ctx context.Context, mqttConnected bool) (service.StateSnapshot, error) {
					snap := sampleSnapshot
					snap.MQTTConnected = mqttConnected
					return snap, nil
				},
			},
			broker:            &mockBrokerChecker{connected: true},
			expectedStatus:    http.StatusOK,
			expectedConnected: true,
		},
		{
			name: "200 OK with disconnected broker",
			provider: &mockStateSnapshotProvider{
				getSnapshotFn: func(ctx context.Context, mqttConnected bool) (service.StateSnapshot, error) {
					snap := sampleSnapshot
					snap.MQTTConnected = mqttConnected
					return snap, nil
				},
			},
			broker:            &mockBrokerChecker{connected: false},
			expectedStatus:    http.StatusOK,
			expectedConnected: false,
		},
		{
			name: "200 OK with nil broker reports mqtt_connected false",
			provider: &mockStateSnapshotProvider{
				getSnapshotFn: func(ctx context.Context, mqttConnected bool) (service.StateSnapshot, error) {
					snap := sampleSnapshot
					snap.MQTTConnected = mqttConnected
					return snap, nil
				},
			},
			broker:            nil,
			expectedStatus:    http.StatusOK,
			expectedConnected: false,
		},
		{
			name:           "503 Service Unavailable when provider is nil",
			provider:       nil,
			broker:         &mockBrokerChecker{connected: true},
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name: "500 Internal Server Error when provider fails",
			provider: &mockStateSnapshotProvider{
				getSnapshotFn: func(ctx context.Context, mqttConnected bool) (service.StateSnapshot, error) {
					return service.StateSnapshot{}, errors.New("db connection failure")
				},
			},
			broker:            &mockBrokerChecker{connected: true},
			expectedStatus:    http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := api.StateHandler(tt.provider, tt.broker)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/state", nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Fatalf("status got %d, want %d. Body: %s", rr.Code, tt.expectedStatus, rr.Body.String())
			}

			if tt.expectedStatus == http.StatusOK {
				var resp service.StateSnapshot
				if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				if resp.MQTTConnected != tt.expectedConnected {
					t.Errorf("expected MQTTConnected %v, got %v", tt.expectedConnected, resp.MQTTConnected)
				}
				if resp.SystemState == nil || resp.SystemState.ID != 1 {
					t.Errorf("expected SystemState ID 1, got %+v", resp.SystemState)
				}
				if len(resp.ShapeCounts) != 3 {
					t.Errorf("expected 3 shape counts, got %d", len(resp.ShapeCounts))
				}
				if len(resp.RecentAudits) != 1 {
					t.Errorf("expected 1 recent audit, got %d", len(resp.RecentAudits))
				}
			}
		})
	}
}
