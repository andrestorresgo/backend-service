package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/andrestorresgo/backend-service/internal/db"
	"github.com/andrestorresgo/backend-service/internal/service"
)

func TestPostgresStateRepository_UninitializedPool(t *testing.T) {
	repo := db.NewPostgresStateRepository(nil)
	ctx := context.Background()

	t.Run("UpdateTelemetry with nil pool", func(t *testing.T) {
		err := repo.UpdateTelemetry(ctx, service.SystemState{}, map[int]int{1: 0}, time.Now())
		if err == nil {
			t.Fatal("expected error with nil pool, got nil")
		}
	})

	t.Run("IncrementRollover with nil pool", func(t *testing.T) {
		err := repo.IncrementRollover(ctx, 1, 5, time.Now())
		if err == nil {
			t.Fatal("expected error with nil pool, got nil")
		}
	})

	t.Run("GetSystemState with nil pool", func(t *testing.T) {
		_, err := repo.GetSystemState(ctx)
		if err == nil {
			t.Fatal("expected error with nil pool, got nil")
		}
	})

	t.Run("GetShapeCounts with nil pool", func(t *testing.T) {
		_, err := repo.GetShapeCounts(ctx)
		if err == nil {
			t.Fatal("expected error with nil pool, got nil")
		}
	})

	t.Run("GetRecentAudits with nil pool", func(t *testing.T) {
		_, err := repo.GetRecentAudits(ctx, 10)
		if err == nil {
			t.Fatal("expected error with nil pool, got nil")
		}
	})
}
