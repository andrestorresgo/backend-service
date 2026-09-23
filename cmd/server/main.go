package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andrestorresgo/backend-service/internal/api"
	"github.com/andrestorresgo/backend-service/internal/config"
	"github.com/andrestorresgo/backend-service/internal/db"
	"github.com/andrestorresgo/backend-service/internal/mqtt"
	"github.com/andrestorresgo/backend-service/internal/service"
)

func main() {
	log.Println("[INFO] Starting backend-service...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Failed to load configuration: %v", err)
	}
	log.Printf("[INFO] Configuration loaded. Server port: %s", cfg.Port)

	var pool *pgxpool.Pool
	if cfg.DatabaseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		var poolErr error
		pool, poolErr = db.NewPool(ctx, cfg.DatabaseURL)
		cancel()

		if poolErr != nil {
			log.Printf("[WARN] Failed to connect to database: %v. Running in degraded state.", poolErr)
		} else {
			defer pool.Close()
			log.Println("[INFO] Connected to PostgreSQL successfully.")

			// Run database migrations on startup
			migrateCtx, migrateCancel := context.WithTimeout(context.Background(), 15*time.Second)
			if err := db.RunMigrations(migrateCtx, pool); err != nil {
				migrateCancel()
				log.Fatalf("[FATAL] Database migration failed: %v", err)
			}
			migrateCancel()
			log.Println("[INFO] Database migrations and initial seed executed successfully.")
		}
	} else {
		log.Println("[WARN] DATABASE_URL is not set. Database features are disabled.")
	}

	var pinger db.DBPinger
	var authService *service.AuthService
	if pool != nil {
		pinger = pool
		authRepo := db.NewPostgresAuthRepository(pool)
		authService = service.NewAuthService(authRepo, service.RealClock{})
	}

	// Initialize MQTT client and workers if broker host is configured
	var mqttClient *mqtt.Client
	var authWorker *mqtt.AuthWorker
	if cfg.MQTTBrokerHost != "" && authService != nil {
		var mqttErr error
		mqttClient, mqttErr = mqtt.NewClient(cfg)
		if mqttErr != nil {
			log.Printf("[WARN] Failed to connect to MQTT broker (%s:%d): %v. Running in degraded state.",
				cfg.MQTTBrokerHost, cfg.MQTTBrokerPort, mqttErr)
		} else {
			authWorker = mqtt.NewAuthWorker(authService, mqttClient, 100)
			authWorker.Start()
			if err := mqttClient.SubscribeAuthRequest(authWorker); err != nil {
				log.Printf("[ERROR] Failed to subscribe auth worker to MQTT: %v", err)
			}
		}
	}

	router := api.NewRouter(cfg, pinger, authService)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Server run context for graceful shutdown
	serverCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("[INFO] Listening on http://localhost:%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[FATAL] HTTP server error: %v", err)
		}
	}()

	<-serverCtx.Done()
	log.Println("[INFO] Shutdown signal received. Draining connections...")

	if authWorker != nil {
		authWorker.Stop()
		log.Println("[INFO] Auth worker stopped.")
	}

	if mqttClient != nil {
		mqttClient.Disconnect(250)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] Server forced to shutdown: %v", err)
	}

	log.Println("[INFO] Server stopped gracefully.")
}
