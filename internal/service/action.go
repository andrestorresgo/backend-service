package service

import (
	"context"
	"encoding/json"
	"time"
)

// TopicActions defines the MQTT topic for broadcasting real-time action log events to the dashboard.
const TopicActions = "factory/actions"

// ActionType defines the category of system action.
type ActionType string

const (
	ActionTypeServo     ActionType = "SERVO"
	ActionTypeMotor     ActionType = "MOTOR"
	ActionTypeDetection ActionType = "DETECTION"
	ActionTypeLockdown  ActionType = "LOCKDOWN"
)

// ActionRecord represents an action log entry for the dashboard action trail.
type ActionRecord struct {
	ID         string     `json:"id"`
	ActionType ActionType `json:"action_type"`
	ActionName string     `json:"action_name"`
	Details    string     `json:"details"`
	Source     string     `json:"source"`
	Timestamp  time.Time  `json:"timestamp"`
}

// ActionRecorder specifies the capability to record system actions in persistence.
type ActionRecorder interface {
	InsertActionLog(ctx context.Context, actionType ActionType, actionName string, details string, source string, timestamp time.Time) error
}

// ActionPublisher specifies an optional interface to broadcast actions to MQTT.
type ActionPublisher interface {
	Publish(topic string, qos byte, retained bool, payload []byte) error
}

// BroadcastAction logs the action to the database recorder and broadcasts to MQTT if publisher is present.
func BroadcastAction(ctx context.Context, recorder ActionRecorder, publisher ActionPublisher, actionType ActionType, actionName string, details string, source string, timestamp time.Time) {
	if recorder != nil {
		_ = recorder.InsertActionLog(ctx, actionType, actionName, details, source, timestamp)
	}

	if publisher != nil {
		rec := ActionRecord{
			ID:         "",
			ActionType: actionType,
			ActionName: actionName,
			Details:    details,
			Source:     source,
			Timestamp:  timestamp,
		}
		if b, err := json.Marshal(rec); err == nil {
			_ = publisher.Publish(TopicActions, 1, false, b)
		}
	}
}
