package mqtt

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"github.com/andrestorresgo/backend-service/internal/service"
)

// TelemetryUpdater defines the domain contract for processing decoded hardware telemetry frames.
type TelemetryUpdater interface {
	UpdateTelemetry(ctx context.Context, data service.TelemetryData) error
}

// TelemetryWorker decouples MQTT telemetry network receive callbacks from database persistence using a buffered channel.
type TelemetryWorker struct {
	updater TelemetryUpdater
	ch      chan []byte
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewTelemetryWorker constructs a TelemetryWorker instance.
func NewTelemetryWorker(updater TelemetryUpdater, bufferSize int) *TelemetryWorker {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &TelemetryWorker{
		updater: updater,
		ch:      make(chan []byte, bufferSize),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start launches the background telemetry processing goroutine.
func (w *TelemetryWorker) Start() {
	w.wg.Add(1)
	go w.run()
}

// Stop signals the worker to exit and awaits completion.
func (w *TelemetryWorker) Stop() {
	w.cancel()
	w.wg.Wait()
}

// HandleMessage safely enqueues an incoming MQTT telemetry payload into the buffered channel.
func (w *TelemetryWorker) HandleMessage(payload []byte) bool {
	data := make([]byte, len(payload))
	copy(data, payload)

	select {
	case w.ch <- data:
		return true
	default:
		log.Println("[WARN] Telemetry worker queue full, dropping message")
		return false
	}
}

func (w *TelemetryWorker) run() {
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

func (w *TelemetryWorker) processPayload(ctx context.Context, data []byte) {
	var p TelemetryPayload
	if err := json.Unmarshal(data, &p); err != nil {
		log.Printf("[WARN] Failed to unmarshal telemetry payload: %v", err)
		return
	}

	telemetryData := service.TelemetryData{
		IsPaused:   p.IsPaused,
		MotorState: p.MotorState,
		ServoState: p.ServoState,
		RedCount:   p.RedCount,
		GreenCount: p.GreenCount,
		BlueCount:  p.BlueCount,
	}

	if err := w.updater.UpdateTelemetry(ctx, telemetryData); err != nil {
		log.Printf("[ERROR] Failed to update telemetry: %v", err)
	}
}

// RolloverProcessor defines the domain contract for processing batch rollover notifications.
type RolloverProcessor interface {
	ProcessRollover(ctx context.Context, data service.RolloverData) error
}

// RolloverWorker decouples MQTT rollover network receive callbacks from database execution using a buffered channel.
type RolloverWorker struct {
	processor RolloverProcessor
	ch        chan []byte
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewRolloverWorker constructs a RolloverWorker instance.
func NewRolloverWorker(processor RolloverProcessor, bufferSize int) *RolloverWorker {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &RolloverWorker{
		processor: processor,
		ch:        make(chan []byte, bufferSize),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start launches the background rollover processing goroutine.
func (w *RolloverWorker) Start() {
	w.wg.Add(1)
	go w.run()
}

// Stop signals the worker to exit and awaits completion.
func (w *RolloverWorker) Stop() {
	w.cancel()
	w.wg.Wait()
}

// HandleMessage safely enqueues an incoming MQTT rollover payload into the buffered channel.
func (w *RolloverWorker) HandleMessage(payload []byte) bool {
	data := make([]byte, len(payload))
	copy(data, payload)

	select {
	case w.ch <- data:
		return true
	default:
		log.Println("[WARN] Rollover worker queue full, dropping message")
		return false
	}
}

func (w *RolloverWorker) run() {
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

func (w *RolloverWorker) processPayload(ctx context.Context, data []byte) {
	var p BatchRolloverPayload
	if err := json.Unmarshal(data, &p); err != nil {
		log.Printf("[WARN] Failed to unmarshal rollover payload: %v", err)
		return
	}

	rolloverData := service.RolloverData{
		ShapeID:   p.ShapeID,
		ShapeName: p.ShapeName,
		Timestamp: p.Timestamp,
	}

	if err := w.processor.ProcessRollover(ctx, rolloverData); err != nil {
		log.Printf("[ERROR] Failed to process rollover: %v", err)
	}
}
