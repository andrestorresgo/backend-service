package mqtt

import (
	"sync"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

type mockPahoToken struct {
	err error
}

func (m *mockPahoToken) Wait() bool {
	return true
}

func (m *mockPahoToken) WaitTimeout(_ time.Duration) bool {
	return true
}

func (m *mockPahoToken) Done() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (m *mockPahoToken) Error() error {
	return m.err
}

type mockPahoClient struct {
	paho.Client
	mu          sync.Mutex
	connected   bool
	subscribed  map[string]byte
	subHandlers map[string]paho.MessageHandler
}

func newMockPahoClient(connected bool) *mockPahoClient {
	return &mockPahoClient{
		connected:   connected,
		subscribed:  make(map[string]byte),
		subHandlers: make(map[string]paho.MessageHandler),
	}
}

func (m *mockPahoClient) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connected
}

func (m *mockPahoClient) Subscribe(topic string, qos byte, callback paho.MessageHandler) paho.Token {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribed[topic] = qos
	m.subHandlers[topic] = callback
	return &mockPahoToken{}
}

func TestClient_SubscribeAndAutoResubscribe(t *testing.T) {
	mockPaho := newMockPahoClient(true)
	client := &Client{
		pahoClient:    mockPaho,
		subscriptions: make(map[string]subscriptionRecord),
	}

	dummyHandler := func(_ paho.Client, _ paho.Message) {}

	// Register subscription
	err := client.subscribeInternal("factory/test/topic", 1, dummyHandler)
	if err != nil {
		t.Fatalf("unexpected error from subscribeInternal: %v", err)
	}

	// Verify it was subscribed initially
	mockPaho.mu.Lock()
	if qos, ok := mockPaho.subscribed["factory/test/topic"]; !ok || qos != 1 {
		mockPaho.mu.Unlock()
		t.Fatalf("expected topic to be subscribed with QoS 1, got %v", qos)
	}
	// Simulate connection loss and wipe on broker (e.g. CleanSession)
	mockPaho.subscribed = make(map[string]byte)
	mockPaho.mu.Unlock()

	// Simulate reconnect triggering resubscribeAll
	client.resubscribeAll(mockPaho)

	// Wait briefly for goroutine in resubscribeAll
	time.Sleep(50 * time.Millisecond)

	mockPaho.mu.Lock()
	defer mockPaho.mu.Unlock()
	if qos, ok := mockPaho.subscribed["factory/test/topic"]; !ok || qos != 1 {
		t.Fatalf("expected topic to be re-subscribed after reconnect with QoS 1, got %v", qos)
	}
}

func TestClient_SubscribeWhenDisconnected_Fails(t *testing.T) {
	mockPaho := newMockPahoClient(false)
	client := &Client{
		pahoClient:    mockPaho,
		subscriptions: make(map[string]subscriptionRecord),
	}

	err := client.subscribeInternal("factory/test/topic", 1, nil)
	if err == nil {
		t.Fatal("expected error when subscribing to disconnected client, got nil")
	}
}
