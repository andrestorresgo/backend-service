package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Servo state constants matching hardware protocol and dashboard expectations.
const (
	ServoStateOpen   = "OPEN"
	ServoStateClosed = "CLOSED"
	TopicActuatorServo = "factory/actuator/servo"

	MotorStateOff    = "OFF"
	MotorStateMedium = "MEDIUM"
	MotorStateOn     = "ON"
	TopicActuatorMotor = "factory/actuator/motor"
)

var (
	// ErrInvalidServoPayload indicates a malformed or missing servo control command.
	ErrInvalidServoPayload = errors.New("invalid servo payload; state must be OPEN or CLOSED, or open must be boolean")
	// ErrInvalidMotorPayload indicates a malformed or missing motor control command.
	ErrInvalidMotorPayload = errors.New("invalid motor payload; state must be ON, MEDIUM, or OFF")
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

// MotorCommandRequest models the incoming request to control the physical DC motor speed.
type MotorCommandRequest struct {
	State *string `json:"state,omitempty"`
	Speed *string `json:"speed,omitempty"`
}

// MotorCommandResult models the response returned upon successful command dispatch.
type MotorCommandResult struct {
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
	publisher       ActuatorPublisher
	recorder        ActionRecorder
	actionPublisher ActionPublisher
	clock           Clock
}

// NewActuatorService constructs a new ActuatorService instance.
func NewActuatorService(pub ActuatorPublisher, recorder ...ActionRecorder) *ActuatorService {
	svc := &ActuatorService{
		publisher: pub,
		clock:     RealClock{},
	}
	if len(recorder) > 0 {
		svc.recorder = recorder[0]
	}
	return svc
}

// SetActionRecorder sets the action recorder and optional clock.
func (s *ActuatorService) SetActionRecorder(recorder ActionRecorder, clock Clock) {
	s.recorder = recorder
	if clock != nil {
		s.clock = clock
	}
}

// SetActionPublisher sets the optional action publisher for MQTT broadcasting.
func (s *ActuatorService) SetActionPublisher(pub ActionPublisher) {
	s.actionPublisher = pub
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

	now := time.Now()
	if s.clock != nil {
		now = s.clock.Now()
	}
	BroadcastAction(ctx, s.recorder, s.actionPublisher, ActionTypeServo, fmt.Sprintf("SERVO_%s", targetState), fmt.Sprintf("Servo gate commanded to %s", targetState), "DASHBOARD", now)

	return ServoCommandResult{
		Status: "dispatched",
		State:  targetState,
	}, nil
}

// CommandMotor validates the requested motor speed state and dispatches the instruction to HiveMQ.
func (s *ActuatorService) CommandMotor(ctx context.Context, req MotorCommandRequest) (MotorCommandResult, error) {
	var targetState string

	if req.State != nil {
		normalized := strings.ToUpper(strings.TrimSpace(*req.State))
		if normalized == MotorStateOn || normalized == MotorStateMedium || normalized == MotorStateOff {
			targetState = normalized
		} else {
			return MotorCommandResult{}, ErrInvalidMotorPayload
		}
	} else if req.Speed != nil {
		normalized := strings.ToUpper(strings.TrimSpace(*req.Speed))
		if normalized == MotorStateOn || normalized == MotorStateMedium || normalized == MotorStateOff {
			targetState = normalized
		} else {
			return MotorCommandResult{}, ErrInvalidMotorPayload
		}
	} else {
		return MotorCommandResult{}, ErrInvalidMotorPayload
	}

	if s.publisher == nil {
		return MotorCommandResult{}, ErrActuatorPublisherUnavailable
	}

	payload := fmt.Sprintf(`{"state":"%s"}`, targetState)
	if err := s.publisher.Publish(TopicActuatorMotor, 1, false, []byte(payload)); err != nil {
		return MotorCommandResult{}, fmt.Errorf("failed to publish motor command to MQTT: %w", err)
	}

	now := time.Now()
	if s.clock != nil {
		now = s.clock.Now()
	}
	BroadcastAction(ctx, s.recorder, s.actionPublisher, ActionTypeMotor, fmt.Sprintf("MOTOR_%s", targetState), fmt.Sprintf("Conveyor DC motor speed commanded to %s", targetState), "DASHBOARD", now)

	return MotorCommandResult{
		Status: "dispatched",
		State:  targetState,
	}, nil
}

