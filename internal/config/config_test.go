package config_test

import (
	"reflect"
	"testing"

	"github.com/andrestorresgo/backend-service/internal/config"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Lookup function returning false for all environment variables
	emptyLookup := func(key string) (string, bool) {
		return "", false
	}

	cfg, err := config.LoadWithLookup(emptyLookup)
	if err != nil {
		t.Fatalf("expected no error loading defaults, got %v", err)
	}

	if cfg.Port != "8080" {
		t.Errorf("expected default Port '8080', got '%s'", cfg.Port)
	}
	if cfg.DatabaseURL != "" {
		t.Errorf("expected default DatabaseURL '', got '%s'", cfg.DatabaseURL)
	}
	if cfg.MQTTBrokerHost != "localhost" {
		t.Errorf("expected default MQTTBrokerHost 'localhost', got '%s'", cfg.MQTTBrokerHost)
	}
	if cfg.MQTTBrokerPort != 8883 {
		t.Errorf("expected default MQTTBrokerPort 8883, got %d", cfg.MQTTBrokerPort)
	}
	if cfg.MQTTClientID != "backend-service-go" {
		t.Errorf("expected default MQTTClientID 'backend-service-go', got '%s'", cfg.MQTTClientID)
	}
	if cfg.VisionBearerToken != "" {
		t.Errorf("expected default VisionBearerToken '', got '%s'", cfg.VisionBearerToken)
	}
	expectedOrigins := []string{"*"}
	if !reflect.DeepEqual(cfg.CORSAllowedOrigins, expectedOrigins) {
		t.Errorf("expected default CORSAllowedOrigins %v, got %v", expectedOrigins, cfg.CORSAllowedOrigins)
	}
}

func TestLoadConfig_CustomValues(t *testing.T) {
	envMap := map[string]string{
		"PORT":                 "9000",
		"DATABASE_URL":         "postgres://user:pass@host:5432/db",
		"MQTT_BROKER_HOST":     "mqtt.hivemq.cloud",
		"MQTT_BROKER_PORT":     "8884",
		"MQTT_USERNAME":        "operator",
		"MQTT_PASSWORD":        "operator-pass",
		"MQTT_CLIENT_ID":       "custom-client-id",
		"VISION_BEARER_TOKEN":  "bearer-secret-xyz",
		"CORS_ALLOWED_ORIGINS": "https://dashboard.example.com,https://staging.example.com",
	}

	lookup := func(key string) (string, bool) {
		val, ok := envMap[key]
		return val, ok
	}

	cfg, err := config.LoadWithLookup(lookup)
	if err != nil {
		t.Fatalf("expected no error loading custom config, got %v", err)
	}

	if cfg.Port != "9000" {
		t.Errorf("expected Port '9000', got '%s'", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://user:pass@host:5432/db" {
		t.Errorf("expected DatabaseURL 'postgres://user:pass@host:5432/db', got '%s'", cfg.DatabaseURL)
	}
	if cfg.MQTTBrokerHost != "mqtt.hivemq.cloud" {
		t.Errorf("expected MQTTBrokerHost 'mqtt.hivemq.cloud', got '%s'", cfg.MQTTBrokerHost)
	}
	if cfg.MQTTBrokerPort != 8884 {
		t.Errorf("expected MQTTBrokerPort 8884, got %d", cfg.MQTTBrokerPort)
	}
	if cfg.MQTTUsername != "operator" {
		t.Errorf("expected MQTTUsername 'operator', got '%s'", cfg.MQTTUsername)
	}
	if cfg.MQTTPassword != "operator-pass" {
		t.Errorf("expected MQTTPassword 'operator-pass', got '%s'", cfg.MQTTPassword)
	}
	if cfg.MQTTClientID != "custom-client-id" {
		t.Errorf("expected MQTTClientID 'custom-client-id', got '%s'", cfg.MQTTClientID)
	}
	if cfg.VisionBearerToken != "bearer-secret-xyz" {
		t.Errorf("expected VisionBearerToken 'bearer-secret-xyz', got '%s'", cfg.VisionBearerToken)
	}
	expectedOrigins := []string{"https://dashboard.example.com", "https://staging.example.com"}
	if !reflect.DeepEqual(cfg.CORSAllowedOrigins, expectedOrigins) {
		t.Errorf("expected CORSAllowedOrigins %v, got %v", expectedOrigins, cfg.CORSAllowedOrigins)
	}
}

func TestLoadConfig_InvalidPort(t *testing.T) {
	envMap := map[string]string{
		"MQTT_BROKER_PORT": "invalid-port",
	}
	lookup := func(key string) (string, bool) {
		val, ok := envMap[key]
		return val, ok
	}

	_, err := config.LoadWithLookup(lookup)
	if err == nil {
		t.Fatal("expected error for invalid MQTT_BROKER_PORT, got nil")
	}
}

func TestLoad(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error loading default config via Load(), got %v", err)
	}
	if cfg.Port == "" {
		t.Error("expected non-empty Port")
	}
}


