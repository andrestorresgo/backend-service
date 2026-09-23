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
	"github.com/andrestorresgo/backend-service/internal/service"
)

type mockActuatorCommander struct {
	commandServoFn func(ctx context.Context, req service.ServoCommandRequest) (service.ServoCommandResult, error)
}

func (m *mockActuatorCommander) CommandServo(ctx context.Context, req service.ServoCommandRequest) (service.ServoCommandResult, error) {
	if m.commandServoFn != nil {
		return m.commandServoFn(ctx, req)
	}
	return service.ServoCommandResult{}, errors.New("not implemented")
}

func TestActuatorServoHandler(t *testing.T) {
	tests := []struct {
		name           string
		commander      api.ActuatorCommander
		payload        string
		expectedStatus int
		expectedState  string
		expectedStatusField string
	}{
		{
			name: "200 OK on state OPEN",
			commander: &mockActuatorCommander{
				commandServoFn: func(ctx context.Context, req service.ServoCommandRequest) (service.ServoCommandResult, error) {
					return service.ServoCommandResult{Status: "dispatched", State: "OPEN"}, nil
				},
			},
			payload:             `{"state": "OPEN"}`,
			expectedStatus:      http.StatusOK,
			expectedState:       "OPEN",
			expectedStatusField: "dispatched",
		},
		{
			name: "200 OK on boolean open false",
			commander: &mockActuatorCommander{
				commandServoFn: func(ctx context.Context, req service.ServoCommandRequest) (service.ServoCommandResult, error) {
					return service.ServoCommandResult{Status: "dispatched", State: "CLOSED"}, nil
				},
			},
			payload:             `{"open": false}`,
			expectedStatus:      http.StatusOK,
			expectedState:       "CLOSED",
			expectedStatusField: "dispatched",
		},
		{
			name: "400 Bad Request on invalid servo state string",
			commander: &mockActuatorCommander{
				commandServoFn: func(ctx context.Context, req service.ServoCommandRequest) (service.ServoCommandResult, error) {
					return service.ServoCommandResult{}, service.ErrInvalidServoPayload
				},
			},
			payload:        `{"state": "AJAR"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "400 Bad Request on malformed JSON",
			commander:      &mockActuatorCommander{},
			payload:        `{invalid json`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "503 Service Unavailable when commander is nil",
			commander:      nil,
			payload:        `{"state": "OPEN"}`,
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name: "503 Service Unavailable when publisher is unavailable",
			commander: &mockActuatorCommander{
				commandServoFn: func(ctx context.Context, req service.ServoCommandRequest) (service.ServoCommandResult, error) {
					return service.ServoCommandResult{}, service.ErrActuatorPublisherUnavailable
				},
			},
			payload:        `{"state": "OPEN"}`,
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name: "500 Internal Server Error on unexpected commander error",
			commander: &mockActuatorCommander{
				commandServoFn: func(ctx context.Context, req service.ServoCommandRequest) (service.ServoCommandResult, error) {
					return service.ServoCommandResult{}, errors.New("unexpected broker error")
				},
			},
			payload:        `{"state": "OPEN"}`,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := api.ActuatorServoHandler(tt.commander)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/actuator/servo", bytes.NewBufferString(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Fatalf("status got %d, want %d. Body: %s", rr.Code, tt.expectedStatus, rr.Body.String())
			}

			if tt.expectedStatus == http.StatusOK {
				var resp service.ServoCommandResult
				if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if resp.Status != tt.expectedStatusField {
					t.Errorf("status field got '%s', want '%s'", resp.Status, tt.expectedStatusField)
				}
				if resp.State != tt.expectedState {
					t.Errorf("state field got '%s', want '%s'", resp.State, tt.expectedState)
				}
			}
		})
	}
}
