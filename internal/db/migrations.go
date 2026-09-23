package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5/pgconn"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// SQLExecutor defines the contract for executing raw SQL queries or migration scripts.
type SQLExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// RunMigrations executes all embedded SQL migration files in lexical order.
func RunMigrations(ctx context.Context, execer SQLExecutor) error {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var filenames []string
	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() != "" {
			filenames = append(filenames, entry.Name())
		}
	}
	sort.Strings(filenames)

	for _, filename := range filenames {
		path := "migrations/" + filename
		content, err := migrationFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", filename, err)
		}

		if _, err := execer.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("failed to execute migration %s: %w", filename, err)
		}
	}

	return nil
}
