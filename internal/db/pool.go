package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DBPinger defines the contract for pinging a database connection.
type DBPinger interface {
	Ping(ctx context.Context) error
}

// NewPool initializes and returns a pgx connection pool.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database configuration: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return pool, nil
}
