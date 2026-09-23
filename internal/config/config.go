package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds the 12-factor application configuration with defaults.
type Config struct {
	Port               string
	DatabaseURL        string
	MQTTBrokerHost     string
	MQTTBrokerPort     int
	MQTTUsername       string
	MQTTPassword       string
	MQTTClientID       string
	VisionBearerToken  string
	CORSAllowedOrigins []string
}

// LookupFunc abstracts environment variable lookups.
type LookupFunc func(string) (string, bool)

// Load reads configuration from .env (if present) and process environment variables.
func Load() (*Config, error) {
	// Attempt to load .env file; ignore error if not found.
	_ = godotenv.Load()

	return LoadWithLookup(os.LookupEnv)
}

// LoadWithLookup builds a Config using the provided lookup function for testing and flexibility.
func LoadWithLookup(lookup LookupFunc) (*Config, error) {
	port := getEnvOrDefault(lookup, "PORT", "8080")
	dbURL := getEnvOrDefault(lookup, "DATABASE_URL", "")
	mqttHost := getEnvOrDefault(lookup, "MQTT_BROKER_HOST", "localhost")
	mqttPortStr := getEnvOrDefault(lookup, "MQTT_BROKER_PORT", "8883")
	mqttUsername := getEnvOrDefault(lookup, "MQTT_USERNAME", "")
	mqttPassword := getEnvOrDefault(lookup, "MQTT_PASSWORD", "")
	mqttClientID := getEnvOrDefault(lookup, "MQTT_CLIENT_ID", "backend-service-go")
	visionToken := getEnvOrDefault(lookup, "VISION_BEARER_TOKEN", "")
	corsOriginsStr := getEnvOrDefault(lookup, "CORS_ALLOWED_ORIGINS", "*")

	mqttPort, err := strconv.Atoi(mqttPortStr)
	if err != nil {
		return nil, fmt.Errorf("invalid MQTT_BROKER_PORT: %w", err)
	}

	var corsOrigins []string
	for _, origin := range strings.Split(corsOriginsStr, ",") {
		trimmed := strings.TrimSpace(origin)
		if trimmed != "" {
			corsOrigins = append(corsOrigins, trimmed)
		}
	}
	if len(corsOrigins) == 0 {
		corsOrigins = []string{"*"}
	}

	return &Config{
		Port:               port,
		DatabaseURL:        dbURL,
		MQTTBrokerHost:     mqttHost,
		MQTTBrokerPort:     mqttPort,
		MQTTUsername:       mqttUsername,
		MQTTPassword:       mqttPassword,
		MQTTClientID:       mqttClientID,
		VisionBearerToken:  visionToken,
		CORSAllowedOrigins: corsOrigins,
	}, nil
}

func getEnvOrDefault(lookup LookupFunc, key, defaultValue string) string {
	if val, ok := lookup(key); ok && val != "" {
		return val
	}
	return defaultValue
}
