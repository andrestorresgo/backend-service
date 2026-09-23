package mqtt_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/andrestorresgo/backend-service/internal/mqtt"
	"github.com/andrestorresgo/backend-service/internal/service"
)

type mockPublisher struct {
	mu        sync.Mutex
	published []publishRecord
}

type publishRecord struct {
	Topic    string
	QoS      byte
	Retained bool
	Payload  []byte
}

func (m *mockPublisher) Publish(topic string, qos byte, retained bool, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published = append(m.published, publishRecord{
		Topic:    topic,
		QoS:      qos,
		Retained: retained,
		Payload:  payload,
	})
	return nil
}

func (m *mockPublisher) getRecords() []publishRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]publishRecord, len(m.published))
	copy(out, m.published)
	return out
}

type mockAuthenticator struct {
	mu           sync.Mutex
	capturedReqs []service.AuthRequest
	authResp     service.AuthResponse
	authErr      error
}

func (m *mockAuthenticator) Authenticate(ctx context.Context, req service.AuthRequest) (service.AuthResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.capturedReqs = append(m.capturedReqs, req)
	if m.authErr != nil {
		return service.AuthResponse{}, m.authErr
	}
	return m.authResp, nil
}

func TestAuthWorker_ProcessValidRequests(t *testing.T) {
	tests := []struct {
		name               string
		incomingPayload    string
		mockResp           service.AuthResponse
		expectedStatus     service.AuthStatus
		expectedUsername   string
		expectedRemaining  int
		expectedLockoutSec int
	}{
		{
			name:            "AUTH_OK matches Board A schema",
			incomingPayload: `{"user_id": 1, "pin": "1234"}`,
			mockResp: service.AuthResponse{
				Status:            service.AuthStatusOK,
				Username:          "Alice",
				RemainingAttempts: 2,
				LockoutSeconds:    0,
			},
			expectedStatus:     service.AuthStatusOK,
			expectedUsername:   "Alice",
			expectedRemaining:  2,
			expectedLockoutSec: 0,
		},
		{
			name:            "INVALID_PIN matches Board A schema",
			incomingPayload: `{"user_id": 1, "pin": "9999"}`,
			mockResp: service.AuthResponse{
				Status:            service.AuthStatusInvalidPIN,
				RemainingAttempts: 1,
				LockoutSeconds:    0,
			},
			expectedStatus:     service.AuthStatusInvalidPIN,
			expectedRemaining:  1,
			expectedLockoutSec: 0,
		},
		{
			name:            "USER_LOCKED matches Board A schema",
			incomingPayload: `{"user_id": 1, "pin": "8888"}`,
			mockResp: service.AuthResponse{
				Status:            service.AuthStatusUserLocked,
				RemainingAttempts: 0,
				LockoutSeconds:    60,
			},
			expectedStatus:     service.AuthStatusUserLocked,
			expectedRemaining:  0,
			expectedLockoutSec: 60,
		},
		{
			name:            "USER_NOT_FOUND matches Board A schema",
			incomingPayload: `{"user_id": 999, "pin": "1234"}`,
			mockResp: service.AuthResponse{
				Status:            service.AuthStatusUserNotFound,
				RemainingAttempts: 0,
				LockoutSeconds:    0,
			},
			expectedStatus:     service.AuthStatusUserNotFound,
			expectedRemaining:  0,
			expectedLockoutSec: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub := &mockPublisher{}
			auth := &mockAuthenticator{authResp: tt.mockResp}

			worker := mqtt.NewAuthWorker(auth, pub, 10)
			worker.Start()

			ok := worker.HandleMessage([]byte(tt.incomingPayload))
			if !ok {
				t.Fatalf("HandleMessage returned false")
			}

			// Wait briefly for worker goroutine to process
			var records []publishRecord
			deadline := time.Now().Add(500 * time.Millisecond)
			for time.Now().Before(deadline) {
				records = pub.getRecords()
				if len(records) > 0 {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			worker.Stop()

			if len(records) != 1 {
				t.Fatalf("expected 1 published message, got %d", len(records))
			}

			record := records[0]
			if record.Topic != mqtt.TopicAuthResponse {
				t.Errorf("topic got %s, want %s", record.Topic, mqtt.TopicAuthResponse)
			}
			if record.QoS != 1 {
				t.Errorf("QoS got %d, want 1", record.QoS)
			}

			var resp service.AuthResponse
			if err := json.Unmarshal(record.Payload, &resp); err != nil {
				t.Fatalf("failed to decode published payload: %v", err)
			}

			if resp.Status != tt.expectedStatus {
				t.Errorf("status got %s, want %s", resp.Status, tt.expectedStatus)
			}
			if resp.Username != tt.expectedUsername {
				t.Errorf("username got %s, want %s", resp.Username, tt.expectedUsername)
			}
			if resp.RemainingAttempts != tt.expectedRemaining {
				t.Errorf("remaining_attempts got %d, want %d", resp.RemainingAttempts, tt.expectedRemaining)
			}
			if resp.LockoutSeconds != tt.expectedLockoutSec {
				t.Errorf("lockout_seconds got %d, want %d", resp.LockoutSeconds, tt.expectedLockoutSec)
			}

			// Verify source passed to Authenticate was KEYPAD
			if len(auth.capturedReqs) != 1 {
				t.Fatalf("expected 1 auth request captured, got %d", len(auth.capturedReqs))
			}
			if auth.capturedReqs[0].Source != service.AuthSourceKeypad {
				t.Errorf("expected source KEYPAD, got %s", auth.capturedReqs[0].Source)
			}
		})
	}
}

func TestAuthWorker_MalformedAndErrorHandling(t *testing.T) {
	tests := []struct {
		name            string
		incomingPayload string
		authErr         error
	}{
		{
			name:            "Broken JSON does not crash or publish",
			incomingPayload: `{not valid json`,
		},
		{
			name:            "Missing pin does not crash or publish",
			incomingPayload: `{"user_id": 1, "pin": ""}`,
		},
		{
			name:            "Invalid user_id does not crash or publish",
			incomingPayload: `{"user_id": 0, "pin": "1234"}`,
		},
		{
			name:            "Service error does not publish",
			incomingPayload: `{"user_id": 1, "pin": "1234"}`,
			authErr:         errors.New("database locked"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub := &mockPublisher{}
			auth := &mockAuthenticator{authErr: tt.authErr}

			worker := mqtt.NewAuthWorker(auth, pub, 10)
			worker.Start()

			worker.HandleMessage([]byte(tt.incomingPayload))

			time.Sleep(50 * time.Millisecond)
			worker.Stop()

			records := pub.getRecords()
			if len(records) != 0 {
				t.Fatalf("expected 0 published messages for error case, got %d", len(records))
			}
		})
	}
}
