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
	if pool != nil {
		pinger = pool
	}
	router := api.NewRouter(cfg, pinger)

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

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] Server forced to shutdown: %v", err)
	}

	log.Println("[INFO] Server stopped gracefully.")
}
