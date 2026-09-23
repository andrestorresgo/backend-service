package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/andrestorresgo/backend-service/internal/service"
)

type MockClock struct {
	currentTime time.Time
}

func (m *MockClock) Now() time.Time {
	return m.currentTime
}

func (m *MockClock) Advance(d time.Duration) {
	m.currentTime = m.currentTime.Add(d)
}

type MockAuthRepo struct {
	mu        sync.Mutex
	users     map[int]*service.User
	auditLogs []service.AuditLogEntry
	withTxErr error
}

func NewMockAuthRepo() *MockAuthRepo {
	return &MockAuthRepo{
		users:     make(map[int]*service.User),
		auditLogs: make([]service.AuditLogEntry, 0),
	}
}

func (m *MockAuthRepo) AddUser(u service.User) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[u.ID] = &service.User{
		ID:             u.ID,
		Username:       u.Username,
		PIN:            u.PIN,
		FailedAttempts: u.FailedAttempts,
		LockedUntil:    u.LockedUntil,
		CreatedAt:      u.CreatedAt,
	}
}

func (m *MockAuthRepo) WithTx(ctx context.Context, fn func(repo service.TxAuthRepository) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.withTxErr != nil {
		return m.withTxErr
	}

	txRepo := &mockTxAuthRepo{repo: m}
	return fn(txRepo)
}

type mockTxAuthRepo struct {
	repo *MockAuthRepo
}

func (m *mockTxAuthRepo) GetUserForUpdate(ctx context.Context, userID int) (*service.User, error) {
	u, ok := m.repo.users[userID]
	if !ok {
		return nil, service.ErrUserNotFound
	}
	// Return a copy so in-place edits require calling UpdateUserSecurityState
	return &service.User{
		ID:             u.ID,
		Username:       u.Username,
		PIN:            u.PIN,
		FailedAttempts: u.FailedAttempts,
		LockedUntil:    u.LockedUntil,
		CreatedAt:      u.CreatedAt,
	}, nil
}

func (m *mockTxAuthRepo) UpdateUserSecurityState(ctx context.Context, userID int, failedAttempts int, lockedUntil *time.Time) error {
	u, ok := m.repo.users[userID]
	if !ok {
		return service.ErrUserNotFound
	}
	u.FailedAttempts = failedAttempts
	u.LockedUntil = lockedUntil
	return nil
}

func (m *mockTxAuthRepo) InsertAuditLog(ctx context.Context, source service.AuthSource, userID *int, status service.AuditStatus, timestamp time.Time) error {
	var copiedID *int
	if userID != nil {
		idVal := *userID
		copiedID = &idVal
	}
	m.repo.auditLogs = append(m.repo.auditLogs, service.AuditLogEntry{
		Source:    source,
		UserID:    copiedID,
		Status:    status,
		Timestamp: timestamp,
	})
	return nil
}

func TestAuthService_Authenticate(t *testing.T) {
	baseTime := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name                 string
		setupUser            *service.User
		requests             []service.AuthRequest
		timeAdvances         []time.Duration
		expectedResponses    []service.AuthResponse
		expectedFailedCounts []int
		expectedAuditCount   int
		expectedLastAudit    *service.AuditLogEntry
	}{
		{
			name: "Correct PIN on first attempt returns AUTH_OK",
			setupUser: &service.User{
				ID:             1,
				Username:       "Andres",
				PIN:            "1234",
				FailedAttempts: 0,
			},
			requests: []service.AuthRequest{
				{UserID: 1, PIN: "1234", Source: service.AuthSourceKeypad},
			},
			timeAdvances: []time.Duration{0},
			expectedResponses: []service.AuthResponse{
				{
					Status:            service.AuthStatusOK,
					Username:          "Andres",
					RemainingAttempts: 2,
					LockoutSeconds:    0,
				},
			},
			expectedFailedCounts: []int{0},
			expectedAuditCount:   1,
			expectedLastAudit: &service.AuditLogEntry{
				Source: service.AuthSourceKeypad,
				UserID: func() *int { id := 1; return &id }(),
				Status: service.AuditStatusSuccess,
			},
		},
		{
			name: "1st failed attempt increments failed_attempts and returns INVALID_PIN",
			setupUser: &service.User{
				ID:             1,
				Username:       "Andres",
				PIN:            "1234",
				FailedAttempts: 0,
			},
			requests: []service.AuthRequest{
				{UserID: 1, PIN: "9999", Source: service.AuthSourceDashboard},
			},
			timeAdvances: []time.Duration{0},
			expectedResponses: []service.AuthResponse{
				{
					Status:            service.AuthStatusInvalidPIN,
					RemainingAttempts: 1,
					LockoutSeconds:    0,
				},
			},
			expectedFailedCounts: []int{1},
			expectedAuditCount:   1,
			expectedLastAudit: &service.AuditLogEntry{
				Source: service.AuthSourceDashboard,
				UserID: func() *int { id := 1; return &id }(),
				Status: service.AuditStatusInvalidPIN,
			},
		},
		{
			name: "2nd consecutive failed attempt triggers 60s lockout and returns USER_LOCKED",
			setupUser: &service.User{
				ID:             1,
				Username:       "Andres",
				PIN:            "1234",
				FailedAttempts: 0,
			},
			requests: []service.AuthRequest{
				{UserID: 1, PIN: "9999", Source: service.AuthSourceKeypad},
				{UserID: 1, PIN: "8888", Source: service.AuthSourceKeypad},
			},
			timeAdvances: []time.Duration{0, 5 * time.Second},
			expectedResponses: []service.AuthResponse{
				{
					Status:            service.AuthStatusInvalidPIN,
					RemainingAttempts: 1,
					LockoutSeconds:    0,
				},
				{
					Status:            service.AuthStatusUserLocked,
					RemainingAttempts: 0,
					LockoutSeconds:    60,
				},
			},
			expectedFailedCounts: []int{1, 2},
			expectedAuditCount:   2,
			expectedLastAudit: &service.AuditLogEntry{
				Source: service.AuthSourceKeypad,
				UserID: func() *int { id := 1; return &id }(),
				Status: service.AuditStatusUserLocked,
			},
		},
		{
			name: "Attempt while actively locked returns USER_LOCKED with remaining seconds",
			setupUser: &service.User{
				ID:             1,
				Username:       "Andres",
				PIN:            "1234",
				FailedAttempts: 2,
				LockedUntil:    func() *time.Time { t := baseTime.Add(60 * time.Second); return &t }(),
			},
			requests: []service.AuthRequest{
				// User attempts 20 seconds into the 60s lockout period
				{UserID: 1, PIN: "1234", Source: service.AuthSourceDashboard},
			},
			timeAdvances: []time.Duration{20 * time.Second},
			expectedResponses: []service.AuthResponse{
				{
					Status:            service.AuthStatusUserLocked,
					RemainingAttempts: 0,
					LockoutSeconds:    40,
				},
			},
			expectedFailedCounts: []int{2},
			expectedAuditCount:   1,
			expectedLastAudit: &service.AuditLogEntry{
				Source: service.AuthSourceDashboard,
				UserID: func() *int { id := 1; return &id }(),
				Status: service.AuditStatusUserLocked,
			},
		},
		{
			name: "Attempt after 60s lockout expires allows successful authentication",
			setupUser: &service.User{
				ID:             1,
				Username:       "Andres",
				PIN:            "1234",
				FailedAttempts: 2,
				LockedUntil:    func() *time.Time { t := baseTime.Add(60 * time.Second); return &t }(),
			},
			requests: []service.AuthRequest{
				// Attempt at 65 seconds (lockout has expired)
				{UserID: 1, PIN: "1234", Source: service.AuthSourceKeypad},
			},
			timeAdvances: []time.Duration{65 * time.Second},
			expectedResponses: []service.AuthResponse{
				{
					Status:            service.AuthStatusOK,
					Username:          "Andres",
					RemainingAttempts: 2,
					LockoutSeconds:    0,
				},
			},
			expectedFailedCounts: []int{0},
			expectedAuditCount:   1,
			expectedLastAudit: &service.AuditLogEntry{
				Source: service.AuthSourceKeypad,
				UserID: func() *int { id := 1; return &id }(),
				Status: service.AuditStatusSuccess,
			},
		},
		{
			name: "Incorrect PIN after lockout expires counts as 1st failure of new cycle",
			setupUser: &service.User{
				ID:             1,
				Username:       "Andres",
				PIN:            "1234",
				FailedAttempts: 2,
				LockedUntil:    func() *time.Time { t := baseTime.Add(60 * time.Second); return &t }(),
			},
			requests: []service.AuthRequest{
				{UserID: 1, PIN: "wrong", Source: service.AuthSourceDashboard},
			},
			timeAdvances: []time.Duration{61 * time.Second},
			expectedResponses: []service.AuthResponse{
				{
					Status:            service.AuthStatusInvalidPIN,
					RemainingAttempts: 1,
					LockoutSeconds:    0,
				},
			},
			expectedFailedCounts: []int{1},
			expectedAuditCount:   1,
			expectedLastAudit: &service.AuditLogEntry{
				Source: service.AuthSourceDashboard,
				UserID: func() *int { id := 1; return &id }(),
				Status: service.AuditStatusInvalidPIN,
			},
		},
		{
			name: "Correct PIN after 1 failure resets failed counter to 0",
			setupUser: &service.User{
				ID:             1,
				Username:       "Andres",
				PIN:            "1234",
				FailedAttempts: 1,
			},
			requests: []service.AuthRequest{
				{UserID: 1, PIN: "1234", Source: service.AuthSourceKeypad},
			},
			timeAdvances: []time.Duration{0},
			expectedResponses: []service.AuthResponse{
				{
					Status:            service.AuthStatusOK,
					Username:          "Andres",
					RemainingAttempts: 2,
					LockoutSeconds:    0,
				},
			},
			expectedFailedCounts: []int{0},
			expectedAuditCount:   1,
			expectedLastAudit: &service.AuditLogEntry{
				Source: service.AuthSourceKeypad,
				UserID: func() *int { id := 1; return &id }(),
				Status: service.AuditStatusSuccess,
			},
		},
		{
			name:      "Non-existent User ID returns USER_NOT_FOUND and logs audit without mutating existing users",
			setupUser: nil,
			requests: []service.AuthRequest{
				{UserID: 999, PIN: "1234", Source: service.AuthSourceKeypad},
			},
			timeAdvances: []time.Duration{0},
			expectedResponses: []service.AuthResponse{
				{
					Status:            service.AuthStatusUserNotFound,
					RemainingAttempts: 0,
					LockoutSeconds:    0,
				},
			},
			expectedFailedCounts: []int{},
			expectedAuditCount:   1,
			expectedLastAudit: &service.AuditLogEntry{
				Source: service.AuthSourceKeypad,
				UserID: nil,
				Status: service.AuditStatusUserNotFound,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockAuthRepo()
			if tt.setupUser != nil {
				repo.AddUser(*tt.setupUser)
			}
			clock := &MockClock{currentTime: baseTime}
			authService := service.NewAuthService(repo, clock)

			ctx := context.Background()

			for i, req := range tt.requests {
				if tt.timeAdvances[i] > 0 {
					clock.currentTime = baseTime.Add(tt.timeAdvances[i])
				}

				resp, err := authService.Authenticate(ctx, req)
				if err != nil {
					t.Fatalf("unexpected error at step %d: %v", i, err)
				}

				expected := tt.expectedResponses[i]
				if resp.Status != expected.Status {
					t.Errorf("step %d status got %s, want %s", i, resp.Status, expected.Status)
				}
				if resp.Username != expected.Username {
					t.Errorf("step %d username got %s, want %s", i, resp.Username, expected.Username)
				}
				if resp.RemainingAttempts != expected.RemainingAttempts {
					t.Errorf("step %d remaining_attempts got %d, want %d", i, resp.RemainingAttempts, expected.RemainingAttempts)
				}
				if resp.LockoutSeconds != expected.LockoutSeconds {
					t.Errorf("step %d lockout_seconds got %d, want %d", i, resp.LockoutSeconds, expected.LockoutSeconds)
				}

				if tt.setupUser != nil && len(tt.expectedFailedCounts) > i {
					u := repo.users[tt.setupUser.ID]
					if u.FailedAttempts != tt.expectedFailedCounts[i] {
						t.Errorf("step %d user failed_attempts got %d, want %d", i, u.FailedAttempts, tt.expectedFailedCounts[i])
					}
				}
			}

			if len(repo.auditLogs) != tt.expectedAuditCount {
				t.Fatalf("audit logs count got %d, want %d", len(repo.auditLogs), tt.expectedAuditCount)
			}

			if tt.expectedLastAudit != nil && len(repo.auditLogs) > 0 {
				lastLog := repo.auditLogs[len(repo.auditLogs)-1]
				if lastLog.Source != tt.expectedLastAudit.Source {
					t.Errorf("audit log source got %s, want %s", lastLog.Source, tt.expectedLastAudit.Source)
				}
				if lastLog.Status != tt.expectedLastAudit.Status {
					t.Errorf("audit log status got %s, want %s", lastLog.Status, tt.expectedLastAudit.Status)
				}
				if tt.expectedLastAudit.UserID == nil && lastLog.UserID != nil {
					t.Errorf("audit log user_id expected nil, got %v", *lastLog.UserID)
				} else if tt.expectedLastAudit.UserID != nil {
					if lastLog.UserID == nil || *lastLog.UserID != *tt.expectedLastAudit.UserID {
						t.Errorf("audit log user_id got %v, want %v", lastLog.UserID, *tt.expectedLastAudit.UserID)
					}
				}
			}
		})
	}
}
