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

type mockDBPinger struct {
	pingErr error
}

func (m *mockDBPinger) Ping(ctx context.Context) error {
	return m.pingErr
}

func TestHealthCheck_Connected(t *testing.T) {
	cfg := &config.Config{
		CORSAllowedOrigins: []string{"*"},
	}
	pinger := &mockDBPinger{pingErr: nil}
	router := api.NewRouter(cfg, pinger, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rr.Code)
	}

	var resp api.HealthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got '%s'", resp.Status)
	}
	if resp.Service != "backend-service" {
		t.Errorf("expected service 'backend-service', got '%s'", resp.Service)
	}
	if resp.Database.Status != "connected" {
		t.Errorf("expected database status 'connected', got '%s'", resp.Database.Status)
	}
}

func TestHealthCheck_DegradedWhenDBFails(t *testing.T) {
	cfg := &config.Config{
		CORSAllowedOrigins: []string{"*"},
	}
	pinger := &mockDBPinger{pingErr: errors.New("connection refused")}
	router := api.NewRouter(cfg, pinger, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rr.Code)
	}

	var resp api.HealthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "degraded" {
		t.Errorf("expected status 'degraded', got '%s'", resp.Status)
	}
	if resp.Database.Status != "disconnected" {
		t.Errorf("expected database status 'disconnected', got '%s'", resp.Database.Status)
	}
	if resp.Database.Error == "" {
		t.Error("expected database error message, got empty string")
	}
}

func TestHealthCheck_NilPinger(t *testing.T) {
	cfg := &config.Config{
		CORSAllowedOrigins: []string{"*"},
	}
	router := api.NewRouter(cfg, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rr.Code)
	}

	var resp api.HealthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "degraded" {
		t.Errorf("expected status 'degraded', got '%s'", resp.Status)
	}
	if resp.Database.Status != "not_configured" {
		t.Errorf("expected database status 'not_configured', got '%s'", resp.Database.Status)
	}
}

func TestHealthCheck_TypedNilPinger(t *testing.T) {
	cfg := &config.Config{
		CORSAllowedOrigins: []string{"*"},
	}
	var typedNil *mockDBPinger = nil
	router := api.NewRouter(cfg, typedNil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rr.Code)
	}

	var resp api.HealthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "degraded" {
		t.Errorf("expected status 'degraded', got '%s'", resp.Status)
	}
	if resp.Database.Status != "not_configured" {
		t.Errorf("expected database status 'not_configured', got '%s'", resp.Database.Status)
	}
}

func TestCORSHeaders(t *testing.T) {
	cfg := &config.Config{
		CORSAllowedOrigins: []string{"https://dashboard.example.com"},
	}
	router := api.NewRouter(cfg, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
	req.Header.Set("Origin", "https://dashboard.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	originHeader := rr.Header().Get("Access-Control-Allow-Origin")
	if originHeader != "https://dashboard.example.com" {
		t.Errorf("expected Access-Control-Allow-Origin 'https://dashboard.example.com', got '%s'", originHeader)
	}
}

func TestPanicRecovery(t *testing.T) {
	cfg := &config.Config{
		CORSAllowedOrigins: []string{"*"},
	}
	router := api.NewRouter(cfg, nil, nil, nil, nil, nil, nil)

	// Add a panicking route to test recoverer middleware
	router.Get("/panic-test", func(w http.ResponseWriter, r *http.Request) {
		panic("simulated catastrophic handler panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic-test", nil)
	rr := httptest.NewRecorder()

	// Should not crash the test process; recoverer catches it and returns 500
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 Internal Server Error after panic, got %d", rr.Code)
	}
}

func TestRouter_DetectionsRoute_AuthenticationAndDispatch(t *testing.T) {
	cfg := &config.Config{
		VisionBearerToken:  "valid-vision-secret-123",
		CORSAllowedOrigins: []string{"*"},
	}

	mockProc := &mockDetectionProcessor{
		processFn: func(ctx context.Context, req service.DetectionRequest) (service.DetectionResult, error) {
			if req.ShapeID == 1 || req.ShapeName == "circle" || req.Shape == "circulo" {
				return service.DetectionResult{
					Status:      service.DetectionStatusDispatched,
					ShapeID:     1,
					DetectionID: 101,
				}, nil
			}
			if req.ShapeID == 2 {
				return service.DetectionResult{
					Status:  service.DetectionStatusDebounced,
					ShapeID: 2,
					Message: "Duplicate detection dropped within 2s debounce window",
				}, nil
			}
			return service.DetectionResult{}, service.ErrInvalidShape
		},
	}

	router := api.NewRouter(cfg, nil, nil, mockProc, nil, nil, nil)

	// 1. Missing Authorization header -> 401
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/detections", bytes.NewReader([]byte(`{"shape_id": 1}`)))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized for missing auth header, got %d", rr.Code)
		}
	}

	// 2. Incorrect Bearer token -> 401
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/detections", bytes.NewReader([]byte(`{"shape_id": 1}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer wrong-token")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized for wrong token, got %d", rr.Code)
		}
	}

	// 3. Valid token and dispatched -> 200 OK
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/detections", bytes.NewReader([]byte(`{"shape_name": "circle"}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-vision-secret-123")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["status"] != "dispatched" || resp["shape_id"] != float64(1) || resp["detection_id"] != float64(101) {
			t.Errorf("unexpected response: %+v", resp)
		}
	}

	// 4. Valid token and debounced -> 200 OK
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/detections", bytes.NewReader([]byte(`{"shape_id": 2}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-vision-secret-123")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["status"] != "debounced" || resp["shape_id"] != float64(2) {
			t.Errorf("unexpected response: %+v", resp)
		}
		if resp["message"] != "Duplicate detection dropped within 2s debounce window" {
			t.Errorf("unexpected message: %v", resp["message"])
		}
	}

	// 5. Valid token, invalid shape -> 400 Bad Request
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/detections", bytes.NewReader([]byte(`{"shape_id": 99}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer valid-vision-secret-123")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for invalid shape, got %d", rr.Code)
		}
	}

	// 6. Route alias /api/vision/detection with valid token, X-Event-ID, and Spanish shape -> 200 OK
	{
		req := httptest.NewRequest(http.MethodPost, "/api/vision/detection", bytes.NewReader([]byte(`{"shape":"circulo","confidence":0.85,"timestamp":"2026-09-23T21:20:15Z"}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Event-ID", "5f805d17-5373-49a8-97a7-4ed6bb7740c0")
		req.Header.Set("Authorization", "Bearer valid-vision-secret-123")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK on /api/vision/detection, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["ok"] != true {
			t.Errorf("expected ok: true, got %v", resp["ok"])
		}
		if resp["status"] != "dispatched" {
			t.Errorf("expected status 'dispatched', got %v", resp["status"])
		}
	}
}

func TestRouter_ActuatorServoRoute_Dispatch(t *testing.T) {
	cfg := &config.Config{
		CORSAllowedOrigins: []string{"*"},
	}

	mockAct := &mockActuatorCommander{
		commandServoFn: func(ctx context.Context, req service.ServoCommandRequest) (service.ServoCommandResult, error) {
			if req.State != nil && *req.State == "OPEN" {
				return service.ServoCommandResult{
					Status: "dispatched",
					State:  "OPEN",
				}, nil
			}
			return service.ServoCommandResult{}, service.ErrInvalidServoPayload
		},
	}

	router := api.NewRouter(cfg, nil, nil, nil, nil, mockAct, nil)

	// Valid command
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/actuator/servo", bytes.NewBufferString(`{"state": "OPEN"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response JSON: %v", err)
		}
		if resp["status"] != "dispatched" || resp["state"] != "OPEN" {
			t.Errorf("unexpected response: %+v", resp)
		}
	}

	// Invalid command
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/actuator/servo", bytes.NewBufferString(`{"state": "INVALID"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400 Bad Request, got %d", rr.Code)
		}
	}
}

func TestRouter_ActuatorMotorRoute_Dispatch(t *testing.T) {
	cfg := &config.Config{
		CORSAllowedOrigins: []string{"*"},
	}

	mockAct := &mockActuatorCommander{
		commandMotorFn: func(ctx context.Context, req service.MotorCommandRequest) (service.MotorCommandResult, error) {
			if req.State != nil && (*req.State == "ON" || *req.State == "MEDIUM" || *req.State == "OFF") {
				return service.MotorCommandResult{
					Status: "dispatched",
					State:  *req.State,
				}, nil
			}
			return service.MotorCommandResult{}, service.ErrInvalidMotorPayload
		},
	}

	router := api.NewRouter(cfg, nil, nil, nil, nil, mockAct, nil)

	// Valid command MEDIUM
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/actuator/motor", bytes.NewBufferString(`{"state": "MEDIUM"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response JSON: %v", err)
		}
		if resp["status"] != "dispatched" || resp["state"] != "MEDIUM" {
			t.Errorf("unexpected response: %+v", resp)
		}
	}

	// Invalid command
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/actuator/motor", bytes.NewBufferString(`{"state": "INVALID"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400 Bad Request, got %d", rr.Code)
		}
	}
}


func TestRouter_StateRoute_ConsolidatedSnapshot(t *testing.T) {
	cfg := &config.Config{
		CORSAllowedOrigins: []string{"*"},
	}

	mockState := &mockStateSnapshotProvider{
		getSnapshotFn: func(ctx context.Context, mqttConnected bool) (service.StateSnapshot, error) {
			return service.StateSnapshot{
				SystemState: &service.SystemState{
					ID:         1,
					IsPaused:   false,
					MotorState: "ON",
					ServoState: false,
				},
				ShapeCounts: []service.ShapeCount{
					{ShapeID: 1, ShapeName: "circle", ColorLabel: "red", LiveBuffer: 1, TotalLifetime: 5},
				},
				RecentAudits:  []service.AuditRecord{},
				MQTTConnected: mqttConnected,
			}, nil
		},
	}

	mockBroker := &mockBrokerChecker{connected: true}
	router := api.NewRouter(cfg, nil, nil, nil, mockState, nil, mockBroker)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/state", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp service.StateSnapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response JSON: %v", err)
	}

	if !resp.MQTTConnected {
		t.Errorf("expected MQTTConnected true, got %v", resp.MQTTConnected)
	}
	if resp.SystemState == nil || resp.SystemState.ID != 1 {
		t.Errorf("expected SystemState ID 1, got %+v", resp.SystemState)
	}
	if len(resp.ShapeCounts) != 1 {
		t.Errorf("expected 1 shape count, got %d", len(resp.ShapeCounts))
	}
}

func TestHealthCheck_MQTTConnected(t *testing.T) {
	cfg := &config.Config{
		CORSAllowedOrigins: []string{"*"},
	}
	pinger := &mockDBPinger{pingErr: nil}
	mockBroker := &mockBrokerChecker{connected: true}
	router := api.NewRouter(cfg, pinger, nil, nil, nil, nil, mockBroker)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rr.Code)
	}

	var resp api.HealthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.MQTT.Status != "connected" {
		t.Errorf("expected mqtt status 'connected', got '%s'", resp.MQTT.Status)
	}
}

