package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

// AuthSource identifies the authentication request origin.
type AuthSource string

const (
	AuthSourceKeypad    AuthSource = "KEYPAD"
	AuthSourceDashboard AuthSource = "DASHBOARD"
)

// AuthStatus defines domain authentication outcome statuses.
type AuthStatus string

const (
	AuthStatusOK           AuthStatus = "AUTH_OK"
	AuthStatusInvalidPIN   AuthStatus = "INVALID_PIN"
	AuthStatusUserLocked   AuthStatus = "USER_LOCKED"
	AuthStatusUserNotFound AuthStatus = "USER_NOT_FOUND"
)

// AuditStatus defines database audit log statuses matching the CHECK constraint.
type AuditStatus string

const (
	AuditStatusSuccess      AuditStatus = "SUCCESS"
	AuditStatusInvalidPIN   AuditStatus = "INVALID_PIN"
	AuditStatusUserLocked   AuditStatus = "USER_LOCKED"
	AuditStatusUserNotFound AuditStatus = "USER_NOT_FOUND"
)

// AuthRequest encapsulates authentication credentials and source.
type AuthRequest struct {
	UserID int        `json:"user_id"`
	PIN    string     `json:"pin"`
	Source AuthSource `json:"source"`
}

// AuthResponse defines the structured response matching Board A protocol and HTTP REST API.
type AuthResponse struct {
	Status            AuthStatus `json:"status"`
	Username          string     `json:"username,omitempty"`
	RemainingAttempts int        `json:"remaining_attempts"`
	LockoutSeconds    int        `json:"lockout_seconds"`
}

// User represents the persistent user credentials and lockout state.
type User struct {
	ID             int
	Username       string
	PIN            string
	FailedAttempts int
	LockedUntil    *time.Time
	CreatedAt      time.Time
}

// AuditLogEntry represents an entry to be appended to auth_audit_logs.
type AuditLogEntry struct {
	Source    AuthSource
	UserID    *int
	Status    AuditStatus
	Timestamp time.Time
}

var ErrUserNotFound = errors.New("user not found")

// Clock abstracts system time for deterministic testing.
type Clock interface {
	Now() time.Time
}

// RealClock provides wall-clock time.
type RealClock struct{}

func (RealClock) Now() time.Time {
	return time.Now()
}

// TxAuthRepository operations executed inside a locked transaction.
type TxAuthRepository interface {
	GetUserForUpdate(ctx context.Context, userID int) (*User, error)
	UpdateUserSecurityState(ctx context.Context, userID int, failedAttempts int, lockedUntil *time.Time) error
	InsertAuditLog(ctx context.Context, source AuthSource, userID *int, status AuditStatus, timestamp time.Time) error
}

// AuthRepository abstracts database transactional boundary.
type AuthRepository interface {
	WithTx(ctx context.Context, fn func(repo TxAuthRepository) error) error
}

// AuthService coordinates authentication logic and lockout enforcement.
type AuthService struct {
	repo      AuthRepository
	clock     Clock
	recorder  ActionRecorder
	publisher ActionPublisher
}

// NewAuthService constructs a new AuthService.
func NewAuthService(repo AuthRepository, clock Clock, recorder ...ActionRecorder) *AuthService {
	if clock == nil {
		clock = RealClock{}
	}
	svc := &AuthService{
		repo:  repo,
		clock: clock,
	}
	if len(recorder) > 0 {
		svc.recorder = recorder[0]
	}
	return svc
}

// SetActionRecorder sets the action recorder.
func (s *AuthService) SetActionRecorder(recorder ActionRecorder) {
	s.recorder = recorder
}

// SetActionPublisher sets the optional action publisher.
func (s *AuthService) SetActionPublisher(pub ActionPublisher) {
	s.publisher = pub
}

// Authenticate verifies credentials inside a row-locked transaction and enforces lockout rules.
func (s *AuthService) Authenticate(ctx context.Context, req AuthRequest) (AuthResponse, error) {
	var resp AuthResponse
	now := s.clock.Now()

	source := req.Source
	if source == "" {
		source = AuthSourceDashboard
	}

	err := s.repo.WithTx(ctx, func(txRepo TxAuthRepository) error {
		user, err := txRepo.GetUserForUpdate(ctx, req.UserID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				// Record attempt with user_id = nil to avoid FK violation, commit audit log
				_ = txRepo.InsertAuditLog(ctx, source, nil, AuditStatusUserNotFound, now)
				resp = AuthResponse{
					Status:            AuthStatusUserNotFound,
					RemainingAttempts: 0,
					LockoutSeconds:    0,
				}
				return nil
			}
			return fmt.Errorf("failed to fetch user for update: %w", err)
		}

		// Active lockout detection: rejects attempts if locked_until > NOW()
		if user.LockedUntil != nil && user.LockedUntil.After(now) {
			remSec := int(math.Ceil(user.LockedUntil.Sub(now).Seconds()))
			if remSec < 1 {
				remSec = 1
			}
			_ = txRepo.InsertAuditLog(ctx, source, &user.ID, AuditStatusUserLocked, now)
			resp = AuthResponse{
				Status:            AuthStatusUserLocked,
				RemainingAttempts: 0,
				LockoutSeconds:    remSec,
			}
			return nil
		}

		// Lockout expiry: if previous lockout expired, reset failed counter for fresh cycle
		failedAttempts := user.FailedAttempts
		if user.LockedUntil != nil && !user.LockedUntil.After(now) {
			failedAttempts = 0
		}

		// Plaintext PIN equality comparison
		if user.PIN == req.PIN {
			// Correct PIN handling: resets failed_attempts = 0, clears locked_until
			if err := txRepo.UpdateUserSecurityState(ctx, user.ID, 0, nil); err != nil {
				return fmt.Errorf("failed to reset user security state: %w", err)
			}
			_ = txRepo.InsertAuditLog(ctx, source, &user.ID, AuditStatusSuccess, now)
			resp = AuthResponse{
				Status:            AuthStatusOK,
				Username:          user.Username,
				RemainingAttempts: 2,
				LockoutSeconds:    0,
			}
			return nil
		}

		// Incorrect PIN handling: increments failed_attempts
		failedAttempts++
		if failedAttempts >= 2 {
			// Reaching 2 consecutive failures locks user for 60 seconds
			lockUntil := now.Add(60 * time.Second)
			if err := txRepo.UpdateUserSecurityState(ctx, user.ID, failedAttempts, &lockUntil); err != nil {
				return fmt.Errorf("failed to apply user lockout: %w", err)
			}
			_ = txRepo.InsertAuditLog(ctx, source, &user.ID, AuditStatusUserLocked, now)
			BroadcastAction(ctx, s.recorder, s.publisher, ActionTypeLockdown, "USER_LOCKOUT", fmt.Sprintf("Operator #%d locked out for 60 seconds after 2 failed PIN attempts", user.ID), string(source), now)
			resp = AuthResponse{
				Status:            AuthStatusUserLocked,
				RemainingAttempts: 0,
				LockoutSeconds:    60,
			}
			return nil
		}

		// 1st failure: 1 remaining attempt
		if err := txRepo.UpdateUserSecurityState(ctx, user.ID, failedAttempts, nil); err != nil {
			return fmt.Errorf("failed to increment failure count: %w", err)
		}
		_ = txRepo.InsertAuditLog(ctx, source, &user.ID, AuditStatusInvalidPIN, now)
		resp = AuthResponse{
			Status:            AuthStatusInvalidPIN,
			RemainingAttempts: 1,
			LockoutSeconds:    0,
		}
		return nil
	})

	return resp, err
}
