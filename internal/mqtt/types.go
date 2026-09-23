package mqtt

// Topic constants
const (
	TopicDetections = "factory/detections"
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
