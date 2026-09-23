package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andrestorresgo/backend-service/internal/api"
	"github.com/andrestorresgo/backend-service/internal/config"
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
	router := api.NewRouter(cfg, pinger, nil)

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
	router := api.NewRouter(cfg, pinger, nil)

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
	router := api.NewRouter(cfg, nil, nil)

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
	router := api.NewRouter(cfg, typedNil, nil)

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
	router := api.NewRouter(cfg, nil, nil)

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
	router := api.NewRouter(cfg, nil, nil)

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

