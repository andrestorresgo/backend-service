package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Servo state constants matching hardware protocol and dashboard expectations.
const (
	ServoStateOpen   = "OPEN"
	ServoStateClosed = "CLOSED"
	TopicActuatorServo = "factory/actuator/servo"
)

var (
	// ErrInvalidServoPayload indicates a malformed or missing servo control command.
	ErrInvalidServoPayload = errors.New("invalid servo payload; state must be OPEN or CLOSED, or open must be boolean")
	// ErrActuatorPublisherUnavailable indicates no MQTT publisher is configured or connected.
	ErrActuatorPublisherUnavailable = errors.New("actuator publisher unavailable")
)

// ServoCommandRequest models the incoming request to control the physical sorting servo gate.
type ServoCommandRequest struct {
	State *string `json:"state,omitempty"`
	Open  *bool   `json:"open,omitempty"`
}

// ServoCommandResult models the response returned upon successful command dispatch.
type ServoCommandResult struct {
	Status string `json:"status"`
	State  string `json:"state"`
}

// ActuatorPublisher abstracts MQTT message publication for actuator commands.
type ActuatorPublisher interface {
	Publish(topic string, qos byte, retained bool, payload []byte) error
}

// ActuatorService coordinates remote physical servo actuation commands.
// Adheres to ADR-0006: publishes command to HiveMQ without optimistically mutating
// the system_state singleton table in PostgreSQL.
type ActuatorService struct {
	publisher ActuatorPublisher
}

// NewActuatorService constructs a new ActuatorService instance.
func NewActuatorService(pub ActuatorPublisher) *ActuatorService {
	return &ActuatorService{publisher: pub}
}

// CommandServo validates the requested servo state and dispatches the instruction to HiveMQ.
func (s *ActuatorService) CommandServo(ctx context.Context, req ServoCommandRequest) (ServoCommandResult, error) {
	var targetState string

	if req.State != nil {
		normalized := strings.ToUpper(strings.TrimSpace(*req.State))
		if normalized == ServoStateOpen || normalized == ServoStateClosed {
			targetState = normalized
		} else {
			return ServoCommandResult{}, ErrInvalidServoPayload
		}
	} else if req.Open != nil {
		if *req.Open {
			targetState = ServoStateOpen
		} else {
			targetState = ServoStateClosed
		}
	} else {
		return ServoCommandResult{}, ErrInvalidServoPayload
	}

	if s.publisher == nil {
		return ServoCommandResult{}, ErrActuatorPublisherUnavailable
	}

	if err := s.publisher.Publish(TopicActuatorServo, 1, false, []byte(targetState)); err != nil {
		return ServoCommandResult{}, fmt.Errorf("failed to publish servo command to MQTT: %w", err)
	}

	return ServoCommandResult{
		Status: "dispatched",
		State:  targetState,
	}, nil
}
