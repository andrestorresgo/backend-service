package mqtt

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/andrestorresgo/backend-service/internal/config"
)

type subscriptionRecord struct {
	topic   string
	qos     byte
	handler paho.MessageHandler
}

// Client wraps Paho MQTT client providing connection management and publishing.
type Client struct {
	pahoClient    paho.Client
	mu            sync.RWMutex
	subscriptions map[string]subscriptionRecord
}

// NewClient initializes and connects an MQTT client configured from application settings.
func NewClient(cfg *config.Config) (*Client, error) {
	if cfg.MQTTBrokerHost == "" {
		return nil, errors.New("mqtt broker host is not configured")
	}

	brokerURI := formatBrokerURI(cfg.MQTTBrokerHost, cfg.MQTTBrokerPort)
	opts := paho.NewClientOptions()
	opts.AddBroker(brokerURI)
	opts.SetClientID(cfg.MQTTClientID)
	opts.SetUsername(cfg.MQTTUsername)
	opts.SetPassword(cfg.MQTTPassword)
	opts.SetAutoReconnect(true)
	opts.SetConnectTimeout(5 * time.Second)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetCleanSession(true)

	// Configure TLS if using a secure scheme or port 8883
	if strings.HasPrefix(brokerURI, "tls://") || strings.HasPrefix(brokerURI, "ssl://") || cfg.MQTTBrokerPort == 8883 {
		opts.SetTLSConfig(&tls.Config{
			MinVersion: tls.VersionTLS12,
		})
	}

	c := &Client{
		subscriptions: make(map[string]subscriptionRecord),
	}

	opts.OnConnect = func(client paho.Client) {
		log.Printf("[INFO] Connected to MQTT broker: %s", brokerURI)
		c.resubscribeAll(client)
	}
	opts.OnConnectionLost = func(_ paho.Client, err error) {
		log.Printf("[WARN] Lost connection to MQTT broker: %v", err)
	}

	client := paho.NewClient(opts)
	token := client.Connect()
	if token.WaitTimeout(10*time.Second) && token.Error() != nil {
		return nil, fmt.Errorf("failed to connect to MQTT broker (%s): %w", brokerURI, token.Error())
	}

	c.pahoClient = client
	return c, nil
}

func (c *Client) resubscribeAll(client paho.Client) {
	c.mu.RLock()
	subs := make([]subscriptionRecord, 0, len(c.subscriptions))
	for _, sub := range c.subscriptions {
		subs = append(subs, sub)
	}
	c.mu.RUnlock()

	for _, sub := range subs {
		s := sub
		token := client.Subscribe(s.topic, s.qos, s.handler)
		go func(rec subscriptionRecord, tok paho.Token) {
			if tok.WaitTimeout(5*time.Second) && tok.Error() != nil {
				log.Printf("[ERROR] Failed to auto-re-subscribe to %s on reconnect: %v", rec.topic, tok.Error())
			} else {
				log.Printf("[INFO] Successfully registered subscription on connect/reconnect: %s", rec.topic)
			}
		}(s, token)
	}
}

func (c *Client) subscribeInternal(topic string, qos byte, handler paho.MessageHandler) error {
	c.mu.Lock()
	c.subscriptions[topic] = subscriptionRecord{
		topic:   topic,
		qos:     qos,
		handler: handler,
	}
	c.mu.Unlock()

	if c.pahoClient == nil || !c.pahoClient.IsConnected() {
		return errors.New("mqtt client is not connected")
	}

	token := c.pahoClient.Subscribe(topic, qos, handler)
	if token.WaitTimeout(5*time.Second) && token.Error() != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", topic, token.Error())
	}

	log.Printf("[INFO] Subscribed to MQTT topic: %s", topic)
	return nil
}

// formatBrokerURI constructs the broker connection URI based on host and port.
func formatBrokerURI(host string, port int) string {
	if strings.Contains(host, "://") {
		return host
	}
	scheme := "tcp"
	if port == 8883 || strings.Contains(host, "hivemq.cloud") {
		scheme = "tls"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, host, port)
}

// Publish satisfies Publisher interface.
func (c *Client) Publish(topic string, qos byte, retained bool, payload []byte) error {
	if c.pahoClient == nil || !c.pahoClient.IsConnected() {
		return errors.New("mqtt client is not connected")
	}

	token := c.pahoClient.Publish(topic, qos, retained, payload)
	if token.WaitTimeout(5*time.Second) && token.Error() != nil {
		return token.Error()
	}
	return nil
}

// SubscribeAuthRequest subscribes the worker to factory/auth/request.
func (c *Client) SubscribeAuthRequest(worker *AuthWorker) error {
	return c.subscribeInternal(TopicAuthRequest, 1, func(_ paho.Client, msg paho.Message) {
		worker.HandleMessage(msg.Payload())
	})
}

// SubscribeTelemetry subscribes the worker to factory/telemetry.
func (c *Client) SubscribeTelemetry(worker *TelemetryWorker) error {
	return c.subscribeInternal(TopicTelemetry, 1, func(_ paho.Client, msg paho.Message) {
		worker.HandleMessage(msg.Payload())
	})
}

// SubscribeRollover subscribes the worker to factory/rollover.
func (c *Client) SubscribeRollover(worker *RolloverWorker) error {
	return c.subscribeInternal(TopicRollover, 1, func(_ paho.Client, msg paho.Message) {
		worker.HandleMessage(msg.Payload())
	})
}


// Disconnect gracefully disconnects from the broker.
func (c *Client) Disconnect(quiesceMs uint) {
	if c.pahoClient != nil && c.pahoClient.IsConnected() {
		c.pahoClient.Disconnect(quiesceMs)
		log.Println("[INFO] Disconnected from MQTT broker.")
	}
}

// IsConnected reports whether the MQTT connection is currently active.
func (c *Client) IsConnected() bool {
	return c.pahoClient != nil && c.pahoClient.IsConnected()
}

// Ensure url package is used if required or avoid unused import
var _ = url.PathEscape
