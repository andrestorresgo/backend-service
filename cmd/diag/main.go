package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/andrestorresgo/backend-service/internal/config"
	paho "github.com/eclipse/paho.mqtt.golang"
)

type ShapeCount struct {
	ShapeID       int    `json:"shape_id"`
	ShapeName     string `json:"shape_name"`
	ColorLabel    string `json:"color_label"`
	LiveBuffer    int    `json:"live_buffer"`
	TotalLifetime int64  `json:"total_lifetime"`
}

type StateResponse struct {
	ShapeCounts []ShapeCount `json:"shape_counts"`
}

func getState() (*StateResponse, error) {
	resp, err := http.Get("https://backend-service-hyc2.onrender.com/api/v1/state")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var s StateResponse
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	stateBefore, err := getState()
	if err != nil {
		log.Fatalf("failed to get state: %v", err)
	}
	fmt.Printf("[DIAG] Before Circle total_lifetime = %d\n", stateBefore.ShapeCounts[0].TotalLifetime)

	opts := paho.NewClientOptions()
	brokerURI := fmt.Sprintf("tls://%s:%d", cfg.MQTTBrokerHost, cfg.MQTTBrokerPort)
	opts.AddBroker(brokerURI)
	opts.SetClientID("diag-tool-runner")
	opts.SetUsername(cfg.MQTTUsername)
	opts.SetPassword(cfg.MQTTPassword)
	opts.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	opts.SetKeepAlive(30 * time.Second)

	client := paho.NewClient(opts)
	if tok := client.Connect(); tok.Wait() && tok.Error() != nil {
		log.Fatalf("MQTT connect failed: %v", tok.Error())
	}
	defer client.Disconnect(250)
	fmt.Println("[DIAG] Connected to MQTT broker!")

	// Listen to rollover topic as well
	client.Subscribe("factory/rollover", 1, func(_ paho.Client, msg paho.Message) {
		fmt.Printf("[DIAG] Heard on factory/rollover: %s\n", string(msg.Payload()))
	})

	time.Sleep(500 * time.Millisecond)

	// Publish rollover
	rolloverPayload := `{"shape_id":1,"shape_name":"circle","timestamp":1700000000}`
	fmt.Println("[DIAG] Publishing to factory/rollover:", rolloverPayload)
	tok := client.Publish("factory/rollover", 1, false, []byte(rolloverPayload))
	tok.Wait()
	if tok.Error() != nil {
		log.Fatalf("publish error: %v", tok.Error())
	}

	fmt.Println("[DIAG] Waiting 4s for backend to process...")
	time.Sleep(4 * time.Second)

	stateAfter, err := getState()
	if err != nil {
		log.Fatalf("failed to get state after: %v", err)
	}
	fmt.Printf("[DIAG] After Circle total_lifetime = %d\n", stateAfter.ShapeCounts[0].TotalLifetime)
	if stateAfter.ShapeCounts[0].TotalLifetime > stateBefore.ShapeCounts[0].TotalLifetime {
		fmt.Printf("[DIAG] SUCCESS: total_lifetime incremented by %d!\n", stateAfter.ShapeCounts[0].TotalLifetime-stateBefore.ShapeCounts[0].TotalLifetime)
	} else {
		fmt.Println("[DIAG] FAILURE: total_lifetime was NOT incremented!")
	}
}
