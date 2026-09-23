package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/andrestorresgo/backend-service/internal/service"
)

// PostgresAuthRepository implements service.AuthRepository using pgxpool.Pool with row-level locks.
type PostgresAuthRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresAuthRepository creates a new PostgresAuthRepository instance.
func NewPostgresAuthRepository(pool *pgxpool.Pool) *PostgresAuthRepository {
	return &PostgresAuthRepository{pool: pool}
}

// WithTx runs an atomic transaction with row-level locking for the authentication lifecycle.
func (r *PostgresAuthRepository) WithTx(ctx context.Context, fn func(repo service.TxAuthRepository) error) error {
	if r.pool == nil {
		return errors.New("database pool is not initialized")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	txRepo := &pgxTxAuthRepo{tx: tx}
	if err := fn(txRepo); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

type pgxTxAuthRepo struct {
	tx pgx.Tx
}

func (r *pgxTxAuthRepo) GetUserForUpdate(ctx context.Context, userID int) (*service.User, error) {
	query := `
		SELECT id, username, pin, failed_attempts, locked_until, created_at
		FROM users
		WHERE id = $1
		FOR UPDATE
	`
	var u service.User
	err := r.tx.QueryRow(ctx, query, userID).Scan(
		&u.ID,
		&u.Username,
		&u.PIN,
		&u.FailedAttempts,
		&u.LockedUntil,
		&u.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, service.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to query user for update: %w", err)
	}
	return &u, nil
}

func (r *pgxTxAuthRepo) UpdateUserSecurityState(ctx context.Context, userID int, failedAttempts int, lockedUntil *time.Time) error {
	query := `
		UPDATE users
		SET failed_attempts = $2, locked_until = $3
		WHERE id = $1
	`
	_, err := r.tx.Exec(ctx, query, userID, failedAttempts, lockedUntil)
	if err != nil {
		return fmt.Errorf("failed to update user security state: %w", err)
	}
	return nil
}

func (r *pgxTxAuthRepo) InsertAuditLog(ctx context.Context, source service.AuthSource, userID *int, status service.AuditStatus, timestamp time.Time) error {
	query := `
		INSERT INTO auth_audit_logs (source, user_id, status, timestamp)
		VALUES ($1, $2, $3, $4)
	`
	_, err := r.tx.Exec(ctx, query, string(source), userID, string(status), timestamp)
	if err != nil {
		return fmt.Errorf("failed to insert auth audit log: %w", err)
	}
	return nil
}
