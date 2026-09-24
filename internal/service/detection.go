package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Canonical Shape Identifiers matching Board A & Board B protocol
const (
	ShapeCircle   uint8 = 1
	ShapeTriangle uint8 = 2
	ShapeSquare   uint8 = 3

	ShapeNameCircle   = "circle"
	ShapeNameTriangle = "triangle"
	ShapeNameSquare   = "square"

	DebounceDuration = 2 * time.Second
	TopicDetections  = "factory/detections"
)

var (
	ErrInvalidShape         = errors.New("invalid or missing shape")
	ErrConflictingShape     = errors.New("conflicting shape_id and shape_name")
	ErrPublisherUnavailable = errors.New("detection publisher unavailable")
)

// DetectionResultStatus indicates whether the detection was dispatched or debounced.
type DetectionResultStatus string

const (
	DetectionStatusDispatched DetectionResultStatus = "dispatched"
	DetectionStatusDebounced  DetectionResultStatus = "debounced"
)

const (
	EventRetentionDuration = 5 * time.Minute
)

// DetectionRequest models incoming computer vision webhook payloads.
type DetectionRequest struct {
	EventID    string  `json:"event_id,omitempty"`
	ShapeID    int     `json:"shape_id"`
	ShapeName  string  `json:"shape_name"`
	Shape      any     `json:"shape"`
	Confidence float64 `json:"confidence,omitempty"`
	Timestamp  string  `json:"timestamp,omitempty"`
}

// DetectionResult represents the API response.
type DetectionResult struct {
	OK          bool                  `json:"ok"`
	Status      DetectionResultStatus `json:"status"`
	ShapeID     int                   `json:"shape_id"`
	DetectionID int                   `json:"detection_id,omitempty"`
	Message     string                `json:"message,omitempty"`
}

// DetectionPublisher abstracts MQTT event publishing.
type DetectionPublisher interface {
	Publish(topic string, qos byte, retained bool, payload []byte) error
}

// DetectionService manages shape resolution, debounce filtering, and MQTT dispatching.
type DetectionService struct {
	publisher       DetectionPublisher
	actionPublisher ActionPublisher
	clock           Clock
	recorder        ActionRecorder
	mu              sync.Mutex
	lastSeen        map[uint8]time.Time
	recentEvents    map[string]time.Time
	detectionID     atomic.Uint32
}

// NewDetectionService constructs a new DetectionService instance.
func NewDetectionService(pub DetectionPublisher, clock Clock, recorder ...ActionRecorder) *DetectionService {
	if clock == nil {
		clock = RealClock{}
	}
	svc := &DetectionService{
		publisher:    pub,
		clock:        clock,
		lastSeen:     make(map[uint8]time.Time),
		recentEvents: make(map[string]time.Time),
	}
	if len(recorder) > 0 {
		svc.recorder = recorder[0]
	}
	return svc
}

// SetActionRecorder sets the action recorder.
func (s *DetectionService) SetActionRecorder(recorder ActionRecorder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recorder = recorder
}

// SetActionPublisher sets the optional action publisher for MQTT broadcasting.
func (s *DetectionService) SetActionPublisher(pub ActionPublisher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actionPublisher = pub
}

// ResolveShape normalizes and validates shape identifiers from a DetectionRequest.
func ResolveShape(req DetectionRequest) (uint8, string, error) {
	var resolvedID uint8
	var resolvedName string

	// 1. Resolve from ShapeID if provided
	if req.ShapeID > 0 {
		switch req.ShapeID {
		case int(ShapeCircle):
			resolvedID = ShapeCircle
			resolvedName = ShapeNameCircle
		case int(ShapeTriangle):
			resolvedID = ShapeTriangle
			resolvedName = ShapeNameTriangle
		case int(ShapeSquare):
			resolvedID = ShapeSquare
			resolvedName = ShapeNameSquare
		default:
			return 0, "", ErrInvalidShape
		}
	}

	// 2. Resolve from ShapeName if provided
	if req.ShapeName != "" {
		idFromName, nameFromName, err := parseShapeString(req.ShapeName)
		if err != nil {
			return 0, "", err
		}
		if resolvedID != 0 && resolvedID != idFromName {
			return 0, "", ErrConflictingShape
		}
		resolvedID = idFromName
		resolvedName = nameFromName
	}

	// 3. Resolve from flexible Shape field if still unresolved
	if resolvedID == 0 && req.Shape != nil {
		switch v := req.Shape.(type) {
		case string:
			idFromStr, nameFromStr, err := parseShapeString(v)
			if err != nil {
				return 0, "", err
			}
			resolvedID = idFromStr
			resolvedName = nameFromStr
		case float64:
			return ResolveShape(DetectionRequest{ShapeID: int(v)})
		case int:
			return ResolveShape(DetectionRequest{ShapeID: v})
		default:
			return 0, "", ErrInvalidShape
		}
	}

	if resolvedID == 0 {
		return 0, "", ErrInvalidShape
	}

	return resolvedID, resolvedName, nil
}

func parseShapeString(raw string) (uint8, string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	switch trimmed {
	case "circle", "red", "circulo", "círculo":
		return ShapeCircle, ShapeNameCircle, nil
	case "triangle", "green", "triangulo", "triángulo":
		return ShapeTriangle, ShapeNameTriangle, nil
	case "square", "blue", "cuadrado":
		return ShapeSquare, ShapeNameSquare, nil
	default:
		// Attempt numeric string parsing
		if id, err := strconv.Atoi(trimmed); err == nil {
			switch uint8(id) {
			case ShapeCircle:
				return ShapeCircle, ShapeNameCircle, nil
			case ShapeTriangle:
				return ShapeTriangle, ShapeNameTriangle, nil
			case ShapeSquare:
				return ShapeSquare, ShapeNameSquare, nil
			}
		}
		return 0, "", ErrInvalidShape
	}
}

// ProcessDetection validates, debounces, and dispatches a shape detection event.
func (s *DetectionService) ProcessDetection(ctx context.Context, req DetectionRequest) (DetectionResult, error) {
	shapeID, shapeName, err := ResolveShape(req)
	if err != nil {
		return DetectionResult{}, err
	}

	now := s.clock.Now()

	s.mu.Lock()
	// 1. Check EventID deduplication if provided (e.g. X-Event-ID header or event_id field)
	if req.EventID != "" {
		if seenAt, exists := s.recentEvents[req.EventID]; exists && now.Sub(seenAt) < EventRetentionDuration {
			s.mu.Unlock()
			return DetectionResult{
				OK:      true,
				Status:  DetectionStatusDebounced,
				ShapeID: int(shapeID),
				Message: "Duplicate event ID dropped",
			}, nil
		}
		s.recentEvents[req.EventID] = now
		if len(s.recentEvents) > 512 {
			for id, t := range s.recentEvents {
				if now.Sub(t) >= EventRetentionDuration {
					delete(s.recentEvents, id)
				}
			}
		}
	}

	// 2. Check 2s debounce window per shape
	lastTime, exists := s.lastSeen[shapeID]
	if exists && now.Sub(lastTime) < DebounceDuration {
		s.mu.Unlock()
		BroadcastAction(ctx, s.recorder, s.actionPublisher, ActionTypeDetection, "DETECTION_DEBOUNCED", fmt.Sprintf("Figure %s (Shape ID %d) dropped (duplicate within 2s debounce window)", shapeName, shapeID), "VISION_SERVICE", now)
		return DetectionResult{
			OK:      true,
			Status:  DetectionStatusDebounced,
			ShapeID: int(shapeID),
			Message: "Duplicate detection dropped within 2s debounce window",
		}, nil
	}

	// Record valid detection timestamp
	s.lastSeen[shapeID] = now
	s.mu.Unlock()

	if s.publisher == nil {
		return DetectionResult{}, ErrPublisherUnavailable
	}

	detID := int(s.detectionID.Add(1))

	// Publish MQTT event matching Board A parser
	payloadMap := map[string]any{
		"shape_id":     int(shapeID),
		"shape_name":   shapeName,
		"detection_id": detID,
	}
	payloadBytes, err := json.Marshal(payloadMap)
	if err != nil {
		return DetectionResult{}, fmt.Errorf("failed to marshal detection payload: %w", err)
	}

	if err := s.publisher.Publish(TopicDetections, 1, false, payloadBytes); err != nil {
		return DetectionResult{}, fmt.Errorf("failed to publish detection to MQTT: %w", err)
	}

	BroadcastAction(ctx, s.recorder, s.actionPublisher, ActionTypeDetection, "FIGURE_DETECTED", fmt.Sprintf("Figure %s (Shape ID %d) detected and dispatched to Actuator", shapeName, shapeID), "VISION_SERVICE", now)

	return DetectionResult{
		OK:          true,
		Status:      DetectionStatusDispatched,
		ShapeID:     int(shapeID),
		DetectionID: detID,
	}, nil
}
