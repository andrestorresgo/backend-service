package mqtt_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/andrestorresgo/backend-service/internal/mqtt"
	"github.com/andrestorresgo/backend-service/internal/service"
)

type mockTelemetryUpdater struct {
	mu           sync.Mutex
	capturedData []service.TelemetryData
	updateErr    error
}

func (m *mockTelemetryUpdater) UpdateTelemetry(ctx context.Context, data service.TelemetryData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.capturedData = append(m.capturedData, data)
	return m.updateErr
}

func (m *mockTelemetryUpdater) getCaptured() []service.TelemetryData {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]service.TelemetryData, len(m.capturedData))
	copy(out, m.capturedData)
	return out
}

type mockRolloverProcessor struct {
	mu           sync.Mutex
	capturedData []service.RolloverData
	processErr   error
}

func (m *mockRolloverProcessor) ProcessRollover(ctx context.Context, data service.RolloverData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.capturedData = append(m.capturedData, data)
	return m.processErr
}

func (m *mockRolloverProcessor) getCaptured() []service.RolloverData {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]service.RolloverData, len(m.capturedData))
	copy(out, m.capturedData)
	return out
}

func TestTelemetryWorker_ProcessValidPayload(t *testing.T) {
	updater := &mockTelemetryUpdater{}
	worker := mqtt.NewTelemetryWorker(updater, 10)
	worker.Start()

	payload := `{"is_paused":true,"motor_state":false,"servo_state":true,"red_count":3,"green_count":1,"blue_count":4}`
	ok := worker.HandleMessage([]byte(payload))
	if !ok {
		t.Fatalf("HandleMessage returned false")
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(updater.getCaptured()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	worker.Stop()

	captured := updater.getCaptured()
	if len(captured) != 1 {
		t.Fatalf("expected 1 telemetry update captured, got %d", len(captured))
	}

	d := captured[0]
	if !d.IsPaused {
		t.Errorf("expected IsPaused true")
	}
	if d.MotorState != "OFF" {
		t.Errorf("expected MotorState OFF, got %s", d.MotorState)
	}
	if !d.ServoState {
		t.Errorf("expected ServoState true")
	}
	if d.RedCount != 3 {
		t.Errorf("expected RedCount 3, got %d", d.RedCount)
	}
	if d.GreenCount != 1 {
		t.Errorf("expected GreenCount 1, got %d", d.GreenCount)
	}
	if d.BlueCount != 4 {
		t.Errorf("expected BlueCount 4, got %d", d.BlueCount)
	}
}

func TestTelemetryWorker_ProcessMotorSpeedStates(t *testing.T) {
	updater := &mockTelemetryUpdater{}
	worker := mqtt.NewTelemetryWorker(updater, 10)
	worker.Start()

	// 1. String "MEDIUM"
	worker.HandleMessage([]byte(`{"is_paused":false,"motor_state":"MEDIUM","servo_state":false,"red_count":0,"green_count":0,"blue_count":0}`))
	// 2. Numeric 1 ("ON")
	worker.HandleMessage([]byte(`{"is_paused":false,"motor_state":1,"servo_state":false,"red_count":0,"green_count":0,"blue_count":0}`))
	// 3. Numeric 2 ("MEDIUM")
	worker.HandleMessage([]byte(`{"is_paused":false,"motor_state":2,"servo_state":false,"red_count":0,"green_count":0,"blue_count":0}`))

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(updater.getCaptured()) >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	worker.Stop()

	captured := updater.getCaptured()
	if len(captured) != 3 {
		t.Fatalf("expected 3 telemetry updates captured, got %d", len(captured))
	}
	if captured[0].MotorState != "MEDIUM" {
		t.Errorf("expected index 0 MotorState MEDIUM, got %s", captured[0].MotorState)
	}
	if captured[1].MotorState != "ON" {
		t.Errorf("expected index 1 MotorState ON, got %s", captured[1].MotorState)
	}
	if captured[2].MotorState != "MEDIUM" {
		t.Errorf("expected index 2 MotorState MEDIUM, got %s", captured[2].MotorState)
	}
}


func TestTelemetryWorker_MalformedJSON(t *testing.T) {
	updater := &mockTelemetryUpdater{}
	worker := mqtt.NewTelemetryWorker(updater, 10)
	worker.Start()

	ok := worker.HandleMessage([]byte(`{not valid json`))
	if !ok {
		t.Fatalf("HandleMessage returned false")
	}

	time.Sleep(50 * time.Millisecond)
	worker.Stop()

	if len(updater.getCaptured()) != 0 {
		t.Fatalf("expected 0 calls for malformed JSON, got %d", len(updater.getCaptured()))
	}
}

func TestTelemetryWorker_QueueFullDrop(t *testing.T) {
	updater := &mockTelemetryUpdater{}
	// Buffer size 1, do not start worker so queue fills immediately
	worker := mqtt.NewTelemetryWorker(updater, 1)

	ok1 := worker.HandleMessage([]byte(`{"is_paused":false}`))
	if !ok1 {
		t.Errorf("expected first message to fit in queue")
	}

	ok2 := worker.HandleMessage([]byte(`{"is_paused":true}`))
	if ok2 {
		t.Errorf("expected second message to be dropped because queue is full")
	}
}

func TestRolloverWorker_ProcessValidPayload(t *testing.T) {
	processor := &mockRolloverProcessor{}
	worker := mqtt.NewRolloverWorker(processor, 10)
	worker.Start()

	payload := `{"shape_id":2,"shape_name":"triangle","timestamp":1695484800}`
	ok := worker.HandleMessage([]byte(payload))
	if !ok {
		t.Fatalf("HandleMessage returned false")
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(processor.getCaptured()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	worker.Stop()

	captured := processor.getCaptured()
	if len(captured) != 1 {
		t.Fatalf("expected 1 rollover captured, got %d", len(captured))
	}

	d := captured[0]
	if d.ShapeID != 2 {
		t.Errorf("expected ShapeID 2, got %d", d.ShapeID)
	}
	if d.ShapeName != "triangle" {
		t.Errorf("expected ShapeName triangle, got %s", d.ShapeName)
	}
	if d.Timestamp != 1695484800 {
		t.Errorf("expected Timestamp 1695484800, got %d", d.Timestamp)
	}
}

func TestRolloverWorker_MalformedJSON(t *testing.T) {
	processor := &mockRolloverProcessor{}
	worker := mqtt.NewRolloverWorker(processor, 10)
	worker.Start()

	ok := worker.HandleMessage([]byte(`invalid json`))
	if !ok {
		t.Fatalf("HandleMessage returned false")
	}

	time.Sleep(50 * time.Millisecond)
	worker.Stop()

	if len(processor.getCaptured()) != 0 {
		t.Fatalf("expected 0 calls for malformed JSON, got %d", len(processor.getCaptured()))
	}
}

func TestRolloverWorker_ServiceError(t *testing.T) {
	processor := &mockRolloverProcessor{processErr: errors.New("db error")}
	worker := mqtt.NewRolloverWorker(processor, 10)
	worker.Start()

	ok := worker.HandleMessage([]byte(`{"shape_id":1,"shape_name":"circle","timestamp":100}`))
	if !ok {
		t.Fatalf("HandleMessage returned false")
	}

	time.Sleep(50 * time.Millisecond)
	worker.Stop()

	if len(processor.getCaptured()) != 1 {
		t.Fatalf("expected processor to be called even if it returns error, got %d calls", len(processor.getCaptured()))
	}
}
