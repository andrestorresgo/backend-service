package mqtt

import (
	"encoding/json"
	"strings"
)

// Topic constants matching hardware and cloud broker configuration
const (
	TopicAuthRequest   = "factory/auth/request"
	TopicAuthResponse  = "factory/auth/response"
	TopicDetections    = "factory/detections"
	TopicTelemetry     = "factory/telemetry"
	TopicRollover      = "factory/rollover"
	TopicActuatorServo = "factory/actuator/servo"
)

// Publisher abstracts MQTT message publication.
type Publisher interface {
	Publish(topic string, qos byte, retained bool, payload []byte) error
}

// KeypadAuthRequest defines the JSON payload received from Board A on factory/auth/request.
type KeypadAuthRequest struct {
	UserID int    `json:"user_id"`
	PIN    string `json:"pin"`
}

// ShapeDetectionMessage defines the outgoing JSON payload published to factory/detections for Board A.
type ShapeDetectionMessage struct {
	ShapeID     int    `json:"shape_id"`
	ShapeName   string `json:"shape_name"`
	DetectionID int    `json:"detection_id"`
}


// TelemetryPayload defines the JSON payload received from Board A on factory/telemetry.
type TelemetryPayload struct {
	IsPaused   bool   `json:"is_paused"`
	MotorState string `json:"motor_state"`
	ServoState bool   `json:"servo_state"`
	RedCount   int    `json:"red_count"`
	GreenCount int    `json:"green_count"`
	BlueCount  int    `json:"blue_count"`
}

// UnmarshalJSON transparently deserializes motor_state from string ("ON", "MEDIUM", "OFF"),
// boolean (true -> "ON", false -> "OFF"), or numeric opcode (1 -> "ON", 2 -> "MEDIUM", 0 -> "OFF").
func (p *TelemetryPayload) UnmarshalJSON(data []byte) error {
	type Alias TelemetryPayload
	aux := struct {
		RawMotorState any `json:"motor_state"`
		*Alias
	}{
		Alias: (*Alias)(p),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	switch v := aux.RawMotorState.(type) {
	case string:
		upper := strings.ToUpper(strings.TrimSpace(v))
		if upper == "ON" || upper == "MEDIUM" || upper == "OFF" {
			p.MotorState = upper
		} else {
			p.MotorState = "OFF"
		}
	case bool:
		if v {
			p.MotorState = "ON"
		} else {
			p.MotorState = "OFF"
		}
	case float64:
		switch int(v) {
		case 1:
			p.MotorState = "ON"
		case 2:
			p.MotorState = "MEDIUM"
		default:
			p.MotorState = "OFF"
		}
	default:
		p.MotorState = "OFF"
	}
	return nil
}

// BatchRolloverPayload defines the JSON payload received from Board A on factory/rollover.
type BatchRolloverPayload struct {
	ShapeID   int    `json:"shape_id"`
	ShapeName string `json:"shape_name"`
	Timestamp int64  `json:"timestamp"`
}


