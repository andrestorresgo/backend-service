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

type mockDetectionProcessor struct {
	processFn func(ctx context.Context, req service.DetectionRequest) (service.DetectionResult, error)
}

func (m *mockDetectionProcessor) ProcessDetection(ctx context.Context, req service.DetectionRequest) (service.DetectionResult, error) {
	if m.processFn != nil {
		return m.processFn(ctx, req)
	}
	return service.DetectionResult{}, nil
}

func TestDetectionHandler_SuccessDispatched(t *testing.T) {
	mockProc := &mockDetectionProcessor{
		processFn: func(ctx context.Context, req service.DetectionRequest) (service.DetectionResult, error) {
			if req.ShapeName != "circle" && req.ShapeID != 1 {
				t.Errorf("unexpected request: %+v", req)
			}
			return service.DetectionResult{
				Status:      service.DetectionStatusDispatched,
				ShapeID:     1,
				DetectionID: 42,
			}, nil
		},
	}

	handler := api.DetectionHandler(mockProc)

	body := []byte(`{"shape_name": "circle"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/detections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["status"] != "dispatched" {
		t.Errorf("expected status 'dispatched', got %v", resp["status"])
	}
	if resp["shape_id"] != float64(1) {
		t.Errorf("expected shape_id 1, got %v", resp["shape_id"])
	}
	if resp["detection_id"] != float64(42) {
		t.Errorf("expected detection_id 42, got %v", resp["detection_id"])
	}
	if _, exists := resp["message"]; exists {
		t.Errorf("message should not be present on dispatched, got %v", resp["message"])
	}
}

func TestDetectionHandler_SuccessDebounced(t *testing.T) {
	mockProc := &mockDetectionProcessor{
		processFn: func(ctx context.Context, req service.DetectionRequest) (service.DetectionResult, error) {
			return service.DetectionResult{
				Status:  service.DetectionStatusDebounced,
				ShapeID: 2,
				Message: "Duplicate detection dropped within 2s debounce window",
			}, nil
		},
	}

	handler := api.DetectionHandler(mockProc)

	body := []byte(`{"shape_id": 2}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/detections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["status"] != "debounced" {
		t.Errorf("expected status 'debounced', got %v", resp["status"])
	}
	if resp["shape_id"] != float64(2) {
		t.Errorf("expected shape_id 2, got %v", resp["shape_id"])
	}
	if resp["message"] != "Duplicate detection dropped within 2s debounce window" {
		t.Errorf("unexpected message: %v", resp["message"])
	}
	if _, exists := resp["detection_id"]; exists {
		t.Errorf("detection_id should not be present on debounced, got %v", resp["detection_id"])
	}
}

func TestDetectionHandler_InvalidJSON(t *testing.T) {
	mockProc := &mockDetectionProcessor{}
	handler := api.DetectionHandler(mockProc)

	body := []byte(`{invalid-json`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/detections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 Bad Request, got %d", rr.Code)
	}
}

func TestDetectionHandler_ValidationError(t *testing.T) {
	mockProc := &mockDetectionProcessor{
		processFn: func(ctx context.Context, req service.DetectionRequest) (service.DetectionResult, error) {
			return service.DetectionResult{}, service.ErrInvalidShape
		},
	}
	handler := api.DetectionHandler(mockProc)

	body := []byte(`{"shape_name": "unknown-octagon"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/detections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 Bad Request, got %d", rr.Code)
	}
}

func TestDetectionHandler_InternalError(t *testing.T) {
	mockProc := &mockDetectionProcessor{
		processFn: func(ctx context.Context, req service.DetectionRequest) (service.DetectionResult, error) {
			return service.DetectionResult{}, errors.New("database or mqtt fatal failure")
		},
	}
	handler := api.DetectionHandler(mockProc)

	body := []byte(`{"shape_id": 1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/detections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500 Internal Server Error, got %d", rr.Code)
	}
}

func TestDetectionHandler_XEventIDHeader(t *testing.T) {
	var capturedReq service.DetectionRequest
	mockProc := &mockDetectionProcessor{
		processFn: func(ctx context.Context, req service.DetectionRequest) (service.DetectionResult, error) {
			capturedReq = req
			return service.DetectionResult{
				OK:          true,
				Status:      service.DetectionStatusDispatched,
				ShapeID:     1,
				DetectionID: 10,
			}, nil
		},
	}
	handler := api.DetectionHandler(mockProc)

	body := []byte(`{"shape": "circulo", "confidence": 0.8462, "timestamp": "2026-09-23T21:20:15.400000+00:00"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/detections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-ID", "5f805d17-5373-49a8-97a7-4ed6bb7740c0")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rr.Code)
	}
	if capturedReq.EventID != "5f805d17-5373-49a8-97a7-4ed6bb7740c0" {
		t.Errorf("expected EventID '5f805d17-5373-49a8-97a7-4ed6bb7740c0', got '%s'", capturedReq.EventID)
	}
	if capturedReq.Shape != "circulo" {
		t.Errorf("expected Shape 'circulo', got '%v'", capturedReq.Shape)
	}
	if capturedReq.Confidence != 0.8462 {
		t.Errorf("expected Confidence 0.8462, got %v", capturedReq.Confidence)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["ok"] != true {
		t.Errorf("expected ok: true, got %v", resp["ok"])
	}
}

