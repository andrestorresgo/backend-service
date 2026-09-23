package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/andrestorresgo/backend-service/internal/service"
)

type mockClock struct {
	mu  sync.Mutex
	now time.Time
}

func newMockClock(initial time.Time) *mockClock {
	return &mockClock{now: initial}
}

func (c *mockClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *mockClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type publishedMessage struct {
	Topic    string
	QoS      byte
	Retained bool
	Payload  []byte
}

type mockPublisher struct {
	mu       sync.Mutex
	messages []publishedMessage
	pubErr   error
}

func (m *mockPublisher) Publish(topic string, qos byte, retained bool, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pubErr != nil {
		return m.pubErr
	}
	m.messages = append(m.messages, publishedMessage{
		Topic:    topic,
		QoS:      qos,
		Retained: retained,
		Payload:  payload,
	})
	return nil
}

func (m *mockPublisher) getMessages() []publishedMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]publishedMessage, len(m.messages))
	copy(copied, m.messages)
	return copied
}

func TestResolveShape(t *testing.T) {
	tests := []struct {
		name         string
		req          service.DetectionRequest
		expectedID   uint8
		expectedName string
		expectErr    bool
	}{
		{
			name:         "ShapeID 1 Circle",
			req:          service.DetectionRequest{ShapeID: 1},
			expectedID:   service.ShapeCircle,
			expectedName: service.ShapeNameCircle,
		},
		{
			name:         "ShapeID 2 Triangle",
			req:          service.DetectionRequest{ShapeID: 2},
			expectedID:   service.ShapeTriangle,
			expectedName: service.ShapeNameTriangle,
		},
		{
			name:         "ShapeID 3 Square",
			req:          service.DetectionRequest{ShapeID: 3},
			expectedID:   service.ShapeSquare,
			expectedName: service.ShapeNameSquare,
		},
		{
			name:         "ShapeName circle lowercase",
			req:          service.DetectionRequest{ShapeName: "circle"},
			expectedID:   service.ShapeCircle,
			expectedName: service.ShapeNameCircle,
		},
		{
			name:         "ShapeName TRIANGLE uppercase",
			req:          service.DetectionRequest{ShapeName: "TRIANGLE"},
			expectedID:   service.ShapeTriangle,
			expectedName: service.ShapeNameTriangle,
		},
		{
			name:         "ShapeName Square mixed case with spaces",
			req:          service.DetectionRequest{ShapeName: "  Square "},
			expectedID:   service.ShapeSquare,
			expectedName: service.ShapeNameSquare,
		},
		{
			name:         "Color red alias",
			req:          service.DetectionRequest{ShapeName: "red"},
			expectedID:   service.ShapeCircle,
			expectedName: service.ShapeNameCircle,
		},
		{
			name:         "Color green alias",
			req:          service.DetectionRequest{ShapeName: "green"},
			expectedID:   service.ShapeTriangle,
			expectedName: service.ShapeNameTriangle,
		},
		{
			name:         "Color blue alias",
			req:          service.DetectionRequest{ShapeName: "blue"},
			expectedID:   service.ShapeSquare,
			expectedName: service.ShapeNameSquare,
		},
		{
			name:         "Shape field string",
			req:          service.DetectionRequest{Shape: "triangle"},
			expectedID:   service.ShapeTriangle,
			expectedName: service.ShapeNameTriangle,
		},
		{
			name:         "Spanish circulo",
			req:          service.DetectionRequest{Shape: "circulo"},
			expectedID:   service.ShapeCircle,
			expectedName: service.ShapeNameCircle,
		},
		{
			name:         "Spanish círculo with accent",
			req:          service.DetectionRequest{Shape: "círculo"},
			expectedID:   service.ShapeCircle,
			expectedName: service.ShapeNameCircle,
		},
		{
			name:         "Spanish triangulo",
			req:          service.DetectionRequest{Shape: "triangulo"},
			expectedID:   service.ShapeTriangle,
			expectedName: service.ShapeNameTriangle,
		},
		{
			name:         "Spanish triángulo with accent",
			req:          service.DetectionRequest{Shape: "triángulo"},
			expectedID:   service.ShapeTriangle,
			expectedName: service.ShapeNameTriangle,
		},
		{
			name:         "Spanish cuadrado",
			req:          service.DetectionRequest{Shape: "cuadrado"},
			expectedID:   service.ShapeSquare,
			expectedName: service.ShapeNameSquare,
		},
		{
			name:         "Shape field float64 from JSON",
			req:          service.DetectionRequest{Shape: float64(3)},
			expectedID:   service.ShapeSquare,
			expectedName: service.ShapeNameSquare,
		},
		{
			name:         "Matching ShapeID and ShapeName",
			req:          service.DetectionRequest{ShapeID: 1, ShapeName: "circle"},
			expectedID:   service.ShapeCircle,
			expectedName: service.ShapeNameCircle,
		},
		{
			name:      "Conflicting ShapeID and ShapeName",
			req:       service.DetectionRequest{ShapeID: 1, ShapeName: "square"},
			expectErr: true,
		},
		{
			name:      "Invalid ShapeID 0",
			req:       service.DetectionRequest{ShapeID: 0},
			expectErr: true,
		},
		{
			name:      "Invalid ShapeID 4",
			req:       service.DetectionRequest{ShapeID: 4},
			expectErr: true,
		},
		{
			name:      "Unknown ShapeName hexagon",
			req:       service.DetectionRequest{ShapeName: "hexagon"},
			expectErr: true,
		},
		{
			name:      "Empty request",
			req:       service.DetectionRequest{},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, name, err := service.ResolveShape(tt.req)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error for request %+v, got id=%d, name=%s", tt.req, id, name)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tt.expectedID {
				t.Errorf("expected id %d, got %d", tt.expectedID, id)
			}
			if name != tt.expectedName {
				t.Errorf("expected name %s, got %s", tt.expectedName, name)
			}
		})
	}
}

func TestDetectionService_DebounceAndDispatch(t *testing.T) {
	startTime := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	clock := newMockClock(startTime)
	pub := &mockPublisher{}

	svc := service.NewDetectionService(pub, clock)

	ctx := context.Background()

	// 1. First Circle detection -> Dispatched
	res1, err := svc.ProcessDetection(ctx, service.DetectionRequest{ShapeName: "circle"})
	if err != nil {
		t.Fatalf("res1 unexpected error: %v", err)
	}
	if res1.Status != service.DetectionStatusDispatched {
		t.Fatalf("expected status 'dispatched', got '%s'", res1.Status)
	}
	if res1.ShapeID != int(service.ShapeCircle) {
		t.Errorf("expected ShapeID 1, got %d", res1.ShapeID)
	}
	if res1.DetectionID != 1 {
		t.Errorf("expected DetectionID 1, got %d", res1.DetectionID)
	}

	msgs := pub.getMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(msgs))
	}
	if msgs[0].Topic != "factory/detections" {
		t.Errorf("expected topic factory/detections, got %s", msgs[0].Topic)
	}

	var payload map[string]any
	if err := json.Unmarshal(msgs[0].Payload, &payload); err != nil {
		t.Fatalf("failed to unmarshal published payload: %v", err)
	}
	if payload["shape_id"] != float64(1) || payload["shape_name"] != "circle" || payload["detection_id"] != float64(1) {
		t.Errorf("unexpected payload content: %+v", payload)
	}

	// 2. Duplicate Circle detection after 1.5 seconds (< 2s) -> Debounced
	clock.Advance(1500 * time.Millisecond)
	res2, err := svc.ProcessDetection(ctx, service.DetectionRequest{ShapeID: 1})
	if err != nil {
		t.Fatalf("res2 unexpected error: %v", err)
	}
	if res2.Status != service.DetectionStatusDebounced {
		t.Fatalf("expected status 'debounced', got '%s'", res2.Status)
	}
	if res2.ShapeID != 1 {
		t.Errorf("expected ShapeID 1, got %d", res2.ShapeID)
	}
	if res2.DetectionID != 0 {
		t.Errorf("expected empty/0 DetectionID on debounced, got %d", res2.DetectionID)
	}
	if res2.Message != "Duplicate detection dropped within 2s debounce window" {
		t.Errorf("unexpected debounce message: %s", res2.Message)
	}

	// Ensure no new message was published
	if len(pub.getMessages()) != 1 {
		t.Fatalf("expected still 1 published message, got %d", len(pub.getMessages()))
	}

	// 3. Different shape (Triangle) at the same time -> Dispatched (independent window)
	resTriangle, err := svc.ProcessDetection(ctx, service.DetectionRequest{ShapeName: "triangle"})
	if err != nil {
		t.Fatalf("resTriangle unexpected error: %v", err)
	}
	if resTriangle.Status != service.DetectionStatusDispatched {
		t.Fatalf("expected triangle status 'dispatched', got '%s'", resTriangle.Status)
	}
	if resTriangle.ShapeID != int(service.ShapeTriangle) {
		t.Errorf("expected ShapeID 2, got %d", resTriangle.ShapeID)
	}
	if resTriangle.DetectionID != 2 {
		t.Errorf("expected DetectionID 2, got %d", resTriangle.DetectionID)
	}
	if len(pub.getMessages()) != 2 {
		t.Fatalf("expected 2 published messages now, got %d", len(pub.getMessages()))
	}

	// 4. Circle detection 2.1s after the original circle detection -> Dispatched
	clock.Advance(600 * time.Millisecond) // now 2.1s since initial circle detection
	res3, err := svc.ProcessDetection(ctx, service.DetectionRequest{ShapeName: "circle"})
	if err != nil {
		t.Fatalf("res3 unexpected error: %v", err)
	}
	if res3.Status != service.DetectionStatusDispatched {
		t.Fatalf("expected status 'dispatched', got '%s'", res3.Status)
	}
	if res3.DetectionID != 3 {
		t.Errorf("expected DetectionID 3, got %d", res3.DetectionID)
	}
	if len(pub.getMessages()) != 3 {
		t.Fatalf("expected 3 published messages, got %d", len(pub.getMessages()))
	}
}

func TestDetectionService_PublisherError(t *testing.T) {
	clock := newMockClock(time.Now())
	pub := &mockPublisher{pubErr: errors.New("mqtt broker connection lost")}

	svc := service.NewDetectionService(pub, clock)

	_, err := svc.ProcessDetection(context.Background(), service.DetectionRequest{ShapeID: 1})
	if err == nil {
		t.Fatal("expected error when publisher fails, got nil")
	}
}

func TestDetectionService_EventIDDeduplication(t *testing.T) {
	ctx := context.Background()
	clock := newMockClock(time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC))
	pub := &mockPublisher{}
	svc := service.NewDetectionService(pub, clock)

	// 1. Initial detection with EventID
	res1, err := svc.ProcessDetection(ctx, service.DetectionRequest{
		EventID: "evt-uuid-1",
		Shape:   "circulo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res1.Status != service.DetectionStatusDispatched {
		t.Fatalf("expected dispatched, got %s", res1.Status)
	}
	if !res1.OK {
		t.Errorf("expected OK to be true")
	}
	if len(pub.getMessages()) != 1 {
		t.Fatalf("expected 1 message, got %d", len(pub.getMessages()))
	}

	// 2. Advance time beyond 2s debounce window (e.g., 5 seconds later, simulating delayed retry)
	clock.Advance(5 * time.Second)

	// 3. Retry with SAME EventID -> must be debounced despite > 2s
	resRetry, err := svc.ProcessDetection(ctx, service.DetectionRequest{
		EventID: "evt-uuid-1",
		Shape:   "circulo",
	})
	if err != nil {
		t.Fatalf("unexpected error on retry: %v", err)
	}
	if resRetry.Status != service.DetectionStatusDebounced {
		t.Fatalf("expected status 'debounced' for duplicate EventID, got '%s'", resRetry.Status)
	}
	if !resRetry.OK {
		t.Errorf("expected OK to be true on debounced retry")
	}
	if len(pub.getMessages()) != 1 {
		t.Fatalf("expected no new published message on retry, still 1, got %d", len(pub.getMessages()))
	}

	// 4. New EventID after retry -> Dispatched
	resNew, err := svc.ProcessDetection(ctx, service.DetectionRequest{
		EventID: "evt-uuid-2",
		Shape:   "circulo",
	})
	if err != nil {
		t.Fatalf("unexpected error on new event: %v", err)
	}
	if resNew.Status != service.DetectionStatusDispatched {
		t.Fatalf("expected dispatched for new EventID, got %s", resNew.Status)
	}
	if len(pub.getMessages()) != 2 {
		t.Fatalf("expected 2 published messages, got %d", len(pub.getMessages()))
	}
}

