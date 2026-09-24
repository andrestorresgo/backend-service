package db_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/andrestorresgo/backend-service/internal/db"
)

type mockSQLExecutor struct {
	executedSQL []string
	execErr     error
}

func (m *mockSQLExecutor) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if m.execErr != nil {
		return pgconn.CommandTag{}, m.execErr
	}
	m.executedSQL = append(m.executedSQL, sql)
	return pgconn.CommandTag{}, nil
}

func TestRunMigrations_Success(t *testing.T) {
	mockExec := &mockSQLExecutor{}
	ctx := context.Background()

	err := db.RunMigrations(ctx, mockExec)
	if err != nil {
		t.Fatalf("expected RunMigrations to succeed, got %v", err)
	}

	if len(mockExec.executedSQL) == 0 {
		t.Fatal("expected at least one SQL execution during migrations")
	}

	fullSQL := strings.Join(mockExec.executedSQL, "\n")

	// Verify required tables are declared
	requiredTables := []string{"users", "system_state", "shape_counts", "auth_audit_logs", "action_logs"}
	for _, table := range requiredTables {
		if !strings.Contains(fullSQL, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Errorf("migration SQL missing table declaration for %s", table)
		}
	}

	// Verify seed data is included
	requiredSeeds := []string{"Andres", "Aldo", "circle", "triangle", "square"}
	for _, seed := range requiredSeeds {
		if !strings.Contains(fullSQL, seed) {
			t.Errorf("migration SQL missing expected seed data '%s'", seed)
		}
	}
}

func TestRunMigrations_ErrorPropagation(t *testing.T) {
	expectedErr := errors.New("db connection failure")
	mockExec := &mockSQLExecutor{execErr: expectedErr}
	ctx := context.Background()

	err := db.RunMigrations(ctx, mockExec)
	if err == nil {
		t.Fatal("expected RunMigrations to return an error, got nil")
	}
	if !errors.Is(err, expectedErr) && !strings.Contains(err.Error(), expectedErr.Error()) {
		t.Errorf("expected error to wrap %v, got %v", expectedErr, err)
	}
}
