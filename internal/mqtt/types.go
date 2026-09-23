package mqtt

// Publisher abstracts MQTT message publication.
type Publisher interface {
	Publish(topic string, qos byte, retained bool, payload []byte) error
}

// KeypadAuthRequest defines the JSON payload received from Board A on factory/auth/request.
type KeypadAuthRequest struct {
	UserID int    `json:"user_id"`
	PIN    string `json:"pin"`
}
