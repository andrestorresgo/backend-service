package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/andrestorresgo/backend-service/internal/config"
	"github.com/andrestorresgo/backend-service/internal/db"
)

func main() {
	log.Println("[INFO] Starting database migration runner...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Failed to load configuration: %v", err)
	}

	if cfg.DatabaseURL == "" {
		log.Fatal("[FATAL] DATABASE_URL is not set in environment or .env file")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	log.Println("[INFO] Connecting to database...")
	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("[FATAL] Database connection failed: %v", err)
	}
	defer pool.Close()

	log.Println("[INFO] Executing migrations...")
	if err := db.RunMigrations(ctx, pool); err != nil {
		log.Fatalf("[FATAL] Migration execution failed: %v", err)
	}
	log.Println("[INFO] Migrations applied successfully!")

	// Verify schema and seed data
	fmt.Println("\n--- Database Verification ---")

	// 1. Verify Users
	var userCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&userCount); err != nil {
		log.Fatalf("[ERROR] Failed to query users table: %v", err)
	}
	fmt.Printf("✓ Table 'users': %d records present\n", userCount)
	rows, err := pool.Query(ctx, "SELECT id, username, pin, failed_attempts FROM users ORDER BY id ASC")
	if err == nil {
		for rows.Next() {
			var id, failed int
			var username, pin string
			_ = rows.Scan(&id, &username, &pin, &failed)
			fmt.Printf("   - User #%d: username=%s, pin=%s, failed_attempts=%d\n", id, username, pin, failed)
		}
		rows.Close()
	}

	// 2. Verify System State
	var id int
	var isPaused, servoState bool
	var motorState string
	if err := pool.QueryRow(ctx, "SELECT id, is_paused, motor_state, servo_state FROM system_state WHERE id = 1").
		Scan(&id, &isPaused, &motorState, &servoState); err != nil {
		log.Fatalf("[ERROR] Failed to query system_state table: %v", err)
	}
	fmt.Printf("✓ Table 'system_state': singleton row present (is_paused=%v, motor_state=%s, servo_state=%v)\n",
		isPaused, motorState, servoState)

	// 3. Verify Shape Counts
	var shapeCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM shape_counts").Scan(&shapeCount); err != nil {
		log.Fatalf("[ERROR] Failed to query shape_counts table: %v", err)
	}
	fmt.Printf("✓ Table 'shape_counts': %d shapes configured\n", shapeCount)
	sRows, err := pool.Query(ctx, "SELECT shape_id, shape_name, color_label, live_buffer, total_lifetime FROM shape_counts ORDER BY shape_id ASC")
	if err == nil {
		for sRows.Next() {
			var shapeID, liveBuffer int
			var shapeName, colorLabel string
			var totalLifetime int64
			_ = sRows.Scan(&shapeID, &shapeName, &colorLabel, &liveBuffer, &totalLifetime)
			fmt.Printf("   - Shape #%d (%s, %s): live_buffer=%d, total_lifetime=%d\n",
				shapeID, shapeName, colorLabel, liveBuffer, totalLifetime)
		}
		sRows.Close()
	}

	// 4. Verify Auth Audit Logs
	var auditCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM auth_audit_logs").Scan(&auditCount); err != nil {
		log.Fatalf("[ERROR] Failed to query auth_audit_logs table: %v", err)
	}
	fmt.Printf("✓ Table 'auth_audit_logs': table verified (%d logs present)\n", auditCount)

	fmt.Println("----------------------------")
	log.Println("[INFO] All tables and seed data verified successfully.")
}
