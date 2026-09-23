package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andrestorresgo/backend-service/internal/service"
)

// PostgresStateRepository implements service.StateRepository using pgxpool.Pool.
type PostgresStateRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresStateRepository creates a new PostgresStateRepository instance.
func NewPostgresStateRepository(pool *pgxpool.Pool) *PostgresStateRepository {
	return &PostgresStateRepository{pool: pool}
}

// UpdateTelemetry updates singleton system_state and synchronizes live_buffer counters inside a single transaction.
func (r *PostgresStateRepository) UpdateTelemetry(ctx context.Context, state service.SystemState, counts map[int]int, updatedAt time.Time) error {
	if r.pool == nil {
		return errors.New("database pool is not initialized")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin telemetry update transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	updateStateSQL := `
		UPDATE system_state
		SET is_paused = $1,
		    motor_state = $2,
		    servo_state = $3,
		    last_telemetry_at = $4
		WHERE id = 1
	`
	_, err = tx.Exec(ctx, updateStateSQL, state.IsPaused, state.MotorState, state.ServoState, updatedAt)
	if err != nil {
		return fmt.Errorf("failed to update system_state: %w", err)
	}

	updateCountSQL := `
		UPDATE shape_counts
		SET live_buffer = $2,
		    updated_at = $3
		WHERE shape_id = $1
	`
	for shapeID, count := range counts {
		_, err = tx.Exec(ctx, updateCountSQL, shapeID, count, updatedAt)
		if err != nil {
			return fmt.Errorf("failed to update live_buffer for shape %d: %w", shapeID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit telemetry transaction: %w", err)
	}

	return nil
}

// IncrementRollover atomically increments total_lifetime by the increment amount and resets live_buffer to 0.
func (r *PostgresStateRepository) IncrementRollover(ctx context.Context, shapeID int, increment int, updatedAt time.Time) error {
	if r.pool == nil {
		return errors.New("database pool is not initialized")
	}

	query := `
		UPDATE shape_counts
		SET total_lifetime = total_lifetime + $2,
		    live_buffer = 0,
		    updated_at = $3
		WHERE shape_id = $1
	`
	tag, err := r.pool.Exec(ctx, query, shapeID, increment, updatedAt)
	if err != nil {
		return fmt.Errorf("failed to execute atomic rollover increment: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return service.ErrShapeNotFound
	}

	return nil
}

// GetSystemState returns the singleton system_state row.
func (r *PostgresStateRepository) GetSystemState(ctx context.Context) (*service.SystemState, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	query := `
		SELECT id, is_paused, motor_state, servo_state, last_telemetry_at
		FROM system_state
		WHERE id = 1
	`
	var s service.SystemState
	err := r.pool.QueryRow(ctx, query).Scan(
		&s.ID,
		&s.IsPaused,
		&s.MotorState,
		&s.ServoState,
		&s.LastTelemetryAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query system_state: %w", err)
	}

	return &s, nil
}

// GetShapeCounts queries all shape counter records ordered by shape_id.
func (r *PostgresStateRepository) GetShapeCounts(ctx context.Context) ([]service.ShapeCount, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	query := `
		SELECT shape_id, shape_name, color_label, live_buffer, total_lifetime, updated_at
		FROM shape_counts
		ORDER BY shape_id ASC
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query shape_counts: %w", err)
	}
	defer rows.Close()

	var counts []service.ShapeCount
	for rows.Next() {
		var sc service.ShapeCount
		if err := rows.Scan(
			&sc.ShapeID,
			&sc.ShapeName,
			&sc.ColorLabel,
			&sc.LiveBuffer,
			&sc.TotalLifetime,
			&sc.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan shape_count row: %w", err)
		}
		counts = append(counts, sc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return counts, nil
}
