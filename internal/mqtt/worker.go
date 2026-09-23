package mqtt

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/andrestorresgo/backend-service/internal/service"
)

const (
	TopicAuthRequest  = "factory/auth/request"
	TopicAuthResponse = "factory/auth/response"
)

// Authenticator defines the domain contract for processing authentication requests.
type Authenticator interface {
	Authenticate(ctx context.Context, req service.AuthRequest) (service.AuthResponse, error)
}

// AuthWorker decouples MQTT network callbacks from database query execution using a buffered channel.
type AuthWorker struct {
	auth      Authenticator
	publisher Publisher
	ch        chan []byte
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewAuthWorker constructs an AuthWorker instance.
func NewAuthWorker(auth Authenticator, pub Publisher, bufferSize int) *AuthWorker {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &AuthWorker{
		auth:      auth,
		publisher: pub,
		ch:        make(chan []byte, bufferSize),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start spawns the background consumer goroutine.
func (w *AuthWorker) Start() {
	w.wg.Add(1)
	go w.run()
}

// Stop signals the worker to exit and awaits termination.
func (w *AuthWorker) Stop() {
	w.cancel()
	w.wg.Wait()
}

// HandleMessage safely enqueues an incoming MQTT message payload into the buffered channel.
func (w *AuthWorker) HandleMessage(payload []byte) bool {
	data := make([]byte, len(payload))
	copy(data, payload)

	select {
	case w.ch <- data:
		return true
	default:
		log.Println("[WARN] Auth worker queue full, dropping message")
		return false
	}
}

func (w *AuthWorker) run() {
	defer w.wg.Done()
	for {
		select {
		case <-w.ctx.Done():
			return
		case data := <-w.ch:
			w.processPayload(w.ctx, data)
		}
	}
}

func (w *AuthWorker) processPayload(ctx context.Context, data []byte) {
	var req KeypadAuthRequest
	if err := json.Unmarshal(data, &req); err != nil {
		log.Printf("[WARN] Failed to unmarshal MQTT auth request: %v", err)
		return
	}

	if req.UserID <= 0 || req.PIN == "" {
		log.Printf("[WARN] Malformed MQTT auth request: user_id=%d, empty pin", req.UserID)
		return
	}

	authReq := service.AuthRequest{
		UserID: req.UserID,
		PIN:    req.PIN,
		Source: service.AuthSourceKeypad,
	}

	resp, err := w.auth.Authenticate(ctx, authReq)
	if err != nil {
		log.Printf("[ERROR] AuthService failed during MQTT processing: %v", err)
		return
	}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal auth response JSON: %v", err)
		return
	}

	if err := w.publisher.Publish(TopicAuthResponse, 1, false, respBytes); err != nil {
		log.Printf("[ERROR] Failed to publish auth response to %s: %v", TopicAuthResponse, err)
	}
}
