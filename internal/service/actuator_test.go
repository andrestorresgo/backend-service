package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/andrestorresgo/backend-service/internal/service"
)

type mockActuatorPublisher struct {
	topic    string
	qos      byte
	retained bool
	payload  []byte
	err      error
}

func (m *mockActuatorPublisher) Publish(topic string, qos byte, retained bool, payload []byte) error {
	if m.err != nil {
		return m.err
	}
	m.topic = topic
	m.qos = qos
	m.retained = retained
	m.payload = payload
	return nil
}

func strPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func TestActuatorService_CommandServo_Success(t *testing.T) {
	tests := []struct {
		name          string
		req           service.ServoCommandRequest
		expectedState string
	}{
		{
			name:          "String OPEN uppercase",
			req:           service.ServoCommandRequest{State: strPtr("OPEN")},
			expectedState: service.ServoStateOpen,
		},
		{
			name:          "String CLOSED uppercase",
			req:           service.ServoCommandRequest{State: strPtr("CLOSED")},
			expectedState: service.ServoStateClosed,
		},
		{
			name:          "String open lowercase with whitespace",
			req:           service.ServoCommandRequest{State: strPtr("  open \n")},
			expectedState: service.ServoStateOpen,
		},
		{
			name:          "String closed mixed case",
			req:           service.ServoCommandRequest{State: strPtr("Closed")},
			expectedState: service.ServoStateClosed,
		},
		{
			name:          "Boolean open true",
			req:           service.ServoCommandRequest{Open: boolPtr(true)},
			expectedState: service.ServoStateOpen,
		},
		{
			name:          "Boolean open false",
			req:           service.ServoCommandRequest{Open: boolPtr(false)},
			expectedState: service.ServoStateClosed,
		},
		{
			name:          "State takes precedence when both provided",
			req:           service.ServoCommandRequest{State: strPtr("CLOSED"), Open: boolPtr(true)},
			expectedState: service.ServoStateClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPub := &mockActuatorPublisher{}
			svc := service.NewActuatorService(mockPub)

			result, err := svc.CommandServo(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Status != "dispatched" {
				t.Errorf("expected status 'dispatched', got '%s'", result.Status)
			}
			if result.State != tt.expectedState {
				t.Errorf("expected state '%s', got '%s'", tt.expectedState, result.State)
			}

			if mockPub.topic != service.TopicActuatorServo {
				t.Errorf("expected topic '%s', got '%s'", service.TopicActuatorServo, mockPub.topic)
			}
			if mockPub.qos != 1 {
				t.Errorf("expected QoS 1, got %d", mockPub.qos)
			}
			if mockPub.retained != false {
				t.Errorf("expected retained false, got %v", mockPub.retained)
			}
			if string(mockPub.payload) != tt.expectedState {
				t.Errorf("expected payload '%s', got '%s'", tt.expectedState, string(mockPub.payload))
			}
		})
	}
}

func TestActuatorService_CommandServo_InvalidPayload(t *testing.T) {
	tests := []struct {
		name string
		req  service.ServoCommandRequest
	}{
		{
			name: "Empty request with nil fields",
			req:  service.ServoCommandRequest{},
		},
		{
			name: "Invalid state string 'AJAR'",
			req:  service.ServoCommandRequest{State: strPtr("AJAR")},
		},
		{
			name: "Empty state string",
			req:  service.ServoCommandRequest{State: strPtr("")},
		},
		{
			name: "Numeric state string",
			req:  service.ServoCommandRequest{State: strPtr("123")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPub := &mockActuatorPublisher{}
			svc := service.NewActuatorService(mockPub)

			_, err := svc.CommandServo(context.Background(), tt.req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, service.ErrInvalidServoPayload) {
				t.Errorf("expected ErrInvalidServoPayload, got %v", err)
			}
		})
	}
}

func TestActuatorService_CommandServo_PublisherUnavailable(t *testing.T) {
	svc := service.NewActuatorService(nil)

	_, err := svc.CommandServo(context.Background(), service.ServoCommandRequest{State: strPtr("OPEN")})
	if err == nil {
		t.Fatal("expected error with nil publisher, got nil")
	}
	if !errors.Is(err, service.ErrActuatorPublisherUnavailable) {
		t.Errorf("expected ErrActuatorPublisherUnavailable, got %v", err)
	}
}

func TestActuatorService_CommandServo_PublishError(t *testing.T) {
	expectedErr := errors.New("network connection timeout")
	mockPub := &mockActuatorPublisher{err: expectedErr}
	svc := service.NewActuatorService(mockPub)

	_, err := svc.CommandServo(context.Background(), service.ServoCommandRequest{State: strPtr("OPEN")})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error to wrap %v, got %v", expectedErr, err)
	}
}

func TestActuatorService_CommandMotor_Success(t *testing.T) {
	tests := []struct {
		name          string
		req           service.MotorCommandRequest
		expectedState string
	}{
		{
			name:          "String ON uppercase",
			req:           service.MotorCommandRequest{State: strPtr("ON")},
			expectedState: service.MotorStateOn,
		},
		{
			name:          "String MEDIUM uppercase",
			req:           service.MotorCommandRequest{State: strPtr("MEDIUM")},
			expectedState: service.MotorStateMedium,
		},
		{
			name:          "String OFF uppercase",
			req:           service.MotorCommandRequest{State: strPtr("OFF")},
			expectedState: service.MotorStateOff,
		},
		{
			name:          "String medium lowercase with whitespace",
			req:           service.MotorCommandRequest{State: strPtr("  medium \n")},
			expectedState: service.MotorStateMedium,
		},
		{
			name:          "String off lowercase",
			req:           service.MotorCommandRequest{State: strPtr("off")},
			expectedState: service.MotorStateOff,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPub := &mockActuatorPublisher{}
			svc := service.NewActuatorService(mockPub)

			res, err := svc.CommandMotor(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if res.Status != "dispatched" {
				t.Errorf("expected status 'dispatched', got '%s'", res.Status)
			}
			if res.State != tt.expectedState {
				t.Errorf("expected state '%s', got '%s'", tt.expectedState, res.State)
			}

			if mockPub.topic != service.TopicActuatorMotor {
				t.Errorf("expected topic '%s', got '%s'", service.TopicActuatorMotor, mockPub.topic)
			}
		})
	}
}

func TestActuatorService_CommandMotor_ValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		req  service.MotorCommandRequest
	}{
		{
			name: "Nil request state",
			req:  service.MotorCommandRequest{},
		},
		{
			name: "Empty string state",
			req:  service.MotorCommandRequest{State: strPtr("")},
		},
		{
			name: "Invalid string state",
			req:  service.MotorCommandRequest{State: strPtr("SUPER_FAST")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPub := &mockActuatorPublisher{}
			svc := service.NewActuatorService(mockPub)

			_, err := svc.CommandMotor(context.Background(), tt.req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, service.ErrInvalidMotorPayload) {
				t.Errorf("expected ErrInvalidMotorPayload, got %v", err)
			}
		})
	}
}

