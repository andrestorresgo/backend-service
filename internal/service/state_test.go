package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/andrestorresgo/backend-service/internal/service"
)

type mockStateRepo struct {
	mu sync.Mutex

	updatedState  *service.SystemState
	updatedCounts map[int]int
	updateTime    time.Time
	updateErr     error

	rolloverShapeID   int
	rolloverIncrement int
	rolloverTime      time.Time
	rolloverErr       error

	systemState  *service.SystemState
	shapeCounts  []service.ShapeCount
	recentAudits []service.AuditRecord
	getErr       error
	auditsErr    error
}

func (m *mockStateRepo) UpdateTelemetry(ctx context.Context, state service.SystemState, counts map[int]int, updatedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updatedState = &state
	m.updatedCounts = make(map[int]int)
	for k, v := range counts {
		m.updatedCounts[k] = v
	}
	m.updateTime = updatedAt
	return nil
}

func (m *mockStateRepo) IncrementRollover(ctx context.Context, shapeID int, increment int, updatedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rolloverErr != nil {
		return m.rolloverErr
	}
	m.rolloverShapeID = shapeID
	m.rolloverIncrement = increment
	m.rolloverTime = updatedAt
	return nil
}

func (m *mockStateRepo) GetSystemState(ctx context.Context) (*service.SystemState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.systemState, nil
}

func (m *mockStateRepo) GetShapeCounts(ctx context.Context) ([]service.ShapeCount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.shapeCounts, nil
}

func (m *mockStateRepo) GetRecentAudits(ctx context.Context, limit int) ([]service.AuditRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.auditsErr != nil {
		return nil, m.auditsErr
	}
	return m.recentAudits, nil
}

func TestStateService_UpdateTelemetry_Success(t *testing.T) {
	fixedTime := time.Date(2026, 9, 23, 15, 0, 0, 0, time.UTC)
	clock := newMockClock(fixedTime)
	repo := &mockStateRepo{}
	svc := service.NewStateService(repo, clock)

	data := service.TelemetryData{
		IsPaused:   true,
		MotorState: false,
		ServoState: true,
		RedCount:   3,
		GreenCount: 4,
		BlueCount:  1,
	}

	err := svc.UpdateTelemetry(context.Background(), data)
	if err != nil {
		t.Fatalf("expected UpdateTelemetry to succeed, got %v", err)
	}

	if repo.updatedState == nil {
		t.Fatal("expected updatedState to be set in repo")
	}

	if repo.updatedState.ID != 1 {
		t.Errorf("expected state ID 1, got %d", repo.updatedState.ID)
	}
	if repo.updatedState.IsPaused != true {
		t.Errorf("expected IsPaused true, got %v", repo.updatedState.IsPaused)
	}
	if repo.updatedState.MotorState != false {
		t.Errorf("expected MotorState false, got %v", repo.updatedState.MotorState)
	}
	if repo.updatedState.ServoState != true {
		t.Errorf("expected ServoState true, got %v", repo.updatedState.ServoState)
	}
	if repo.updatedState.LastTelemetryAt == nil || !repo.updatedState.LastTelemetryAt.Equal(fixedTime) {
		t.Errorf("expected LastTelemetryAt %v, got %v", fixedTime, repo.updatedState.LastTelemetryAt)
	}

	// Verify live buffers: red -> 1 (Circle), green -> 2 (Triangle), blue -> 3 (Square)
	if repo.updatedCounts[service.ShapeCircleID] != 3 {
		t.Errorf("expected Circle live buffer 3, got %d", repo.updatedCounts[service.ShapeCircleID])
	}
	if repo.updatedCounts[service.ShapeTriangleID] != 4 {
		t.Errorf("expected Triangle live buffer 4, got %d", repo.updatedCounts[service.ShapeTriangleID])
	}
	if repo.updatedCounts[service.ShapeSquareID] != 1 {
		t.Errorf("expected Square live buffer 1, got %d", repo.updatedCounts[service.ShapeSquareID])
	}
	if !repo.updateTime.Equal(fixedTime) {
		t.Errorf("expected updateTime %v, got %v", fixedTime, repo.updateTime)
	}
}

func TestStateService_UpdateTelemetry_Clamping(t *testing.T) {
	fixedTime := time.Date(2026, 9, 23, 15, 0, 0, 0, time.UTC)
	clock := newMockClock(fixedTime)
	repo := &mockStateRepo{}
	svc := service.NewStateService(repo, clock)

	data := service.TelemetryData{
		IsPaused:   false,
		MotorState: true,
		ServoState: false,
		RedCount:   -10, // should clamp to 0
		GreenCount: 6,   // should clamp to 5
		BlueCount:  99,  // should clamp to 5
	}

	err := svc.UpdateTelemetry(context.Background(), data)
	if err != nil {
		t.Fatalf("expected UpdateTelemetry to succeed with clamping, got %v", err)
	}

	if repo.updatedCounts[service.ShapeCircleID] != 0 {
		t.Errorf("expected Circle live buffer clamped to 0, got %d", repo.updatedCounts[service.ShapeCircleID])
	}
	if repo.updatedCounts[service.ShapeTriangleID] != 5 {
		t.Errorf("expected Triangle live buffer clamped to 5, got %d", repo.updatedCounts[service.ShapeTriangleID])
	}
	if repo.updatedCounts[service.ShapeSquareID] != 5 {
		t.Errorf("expected Square live buffer clamped to 5, got %d", repo.updatedCounts[service.ShapeSquareID])
	}
}

func TestStateService_UpdateTelemetry_RepoError(t *testing.T) {
	clock := newMockClock(time.Now())
	expectedErr := errors.New("db write failed")
	repo := &mockStateRepo{updateErr: expectedErr}
	svc := service.NewStateService(repo, clock)

	err := svc.UpdateTelemetry(context.Background(), service.TelemetryData{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error to wrap %v, got %v", expectedErr, err)
	}
}

func TestStateService_ProcessRollover_SuccessWithShapeID(t *testing.T) {
	fixedTime := time.Date(2026, 9, 23, 15, 30, 0, 0, time.UTC)
	clock := newMockClock(fixedTime)
	repo := &mockStateRepo{}
	svc := service.NewStateService(repo, clock)

	tests := []struct {
		name     string
		shapeID  int
		expected int
	}{
		{"Circle", 1, 1},
		{"Triangle", 2, 2},
		{"Square", 3, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.ProcessRollover(context.Background(), service.RolloverData{
				ShapeID:   tt.shapeID,
				Timestamp: fixedTime.Unix(),
			})
			if err != nil {
				t.Fatalf("expected ProcessRollover to succeed, got %v", err)
			}
			if repo.rolloverShapeID != tt.expected {
				t.Errorf("expected rolloverShapeID %d, got %d", tt.expected, repo.rolloverShapeID)
			}
			if repo.rolloverIncrement != service.RolloverBatchIncrement {
				t.Errorf("expected increment %d, got %d", service.RolloverBatchIncrement, repo.rolloverIncrement)
			}
			if !repo.rolloverTime.Equal(fixedTime) {
				t.Errorf("expected rolloverTime %v, got %v", fixedTime, repo.rolloverTime)
			}
		})
	}
}

func TestStateService_ProcessRollover_ResolveByName(t *testing.T) {
	fixedTime := time.Date(2026, 9, 23, 15, 30, 0, 0, time.UTC)
	clock := newMockClock(fixedTime)
	repo := &mockStateRepo{}
	svc := service.NewStateService(repo, clock)

	tests := []struct {
		name      string
		shapeName string
		expected  int
	}{
		{"Circle lowercase", "circle", service.ShapeCircleID},
		{"Circle uppercase", "CIRCLE", service.ShapeCircleID},
		{"Red color alias", "red", service.ShapeCircleID},
		{"Triangle lowercase", "triangle", service.ShapeTriangleID},
		{"Green color alias", "GREEN", service.ShapeTriangleID},
		{"Square lowercase", "square", service.ShapeSquareID},
		{"Blue color alias", "blue", service.ShapeSquareID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.ProcessRollover(context.Background(), service.RolloverData{
				ShapeID:   0,
				ShapeName: tt.shapeName,
			})
			if err != nil {
				t.Fatalf("expected ProcessRollover to resolve %s, got err %v", tt.shapeName, err)
			}
			if repo.rolloverShapeID != tt.expected {
				t.Errorf("expected shape ID %d for %s, got %d", tt.expected, tt.shapeName, repo.rolloverShapeID)
			}
		})
	}
}

func TestStateService_ProcessRollover_InvalidShape(t *testing.T) {
	clock := newMockClock(time.Now())
	repo := &mockStateRepo{}
	svc := service.NewStateService(repo, clock)

	tests := []struct {
		name string
		data service.RolloverData
	}{
		{"Zero shape ID with unknown name", service.RolloverData{ShapeID: 0, ShapeName: "hexagon"}},
		{"Zero shape ID with empty name", service.RolloverData{ShapeID: 0, ShapeName: ""}},
		{"Out of range shape ID high", service.RolloverData{ShapeID: 4}},
		{"Out of range shape ID negative", service.RolloverData{ShapeID: -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.ProcessRollover(context.Background(), tt.data)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
			if !errors.Is(err, service.ErrInvalidShapeID) {
				t.Errorf("expected ErrInvalidShapeID, got %v", err)
			}
		})
	}
}

func TestStateService_ProcessRollover_RepoError(t *testing.T) {
	clock := newMockClock(time.Now())
	expectedErr := service.ErrShapeNotFound
	repo := &mockStateRepo{rolloverErr: expectedErr}
	svc := service.NewStateService(repo, clock)

	err := svc.ProcessRollover(context.Background(), service.RolloverData{ShapeID: 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error to wrap %v, got %v", expectedErr, err)
	}
}

func TestStateService_GetSystemState_And_GetShapeCounts(t *testing.T) {
	clock := newMockClock(time.Now())
	expectedState := &service.SystemState{
		ID:         1,
		IsPaused:   false,
		MotorState: true,
		ServoState: true,
	}
	expectedCounts := []service.ShapeCount{
		{ShapeID: 1, ShapeName: "circle", ColorLabel: "red", LiveBuffer: 2, TotalLifetime: 10},
		{ShapeID: 2, ShapeName: "triangle", ColorLabel: "green", LiveBuffer: 0, TotalLifetime: 5},
		{ShapeID: 3, ShapeName: "square", ColorLabel: "blue", LiveBuffer: 4, TotalLifetime: 15},
	}

	repo := &mockStateRepo{
		systemState: expectedState,
		shapeCounts: expectedCounts,
	}
	svc := service.NewStateService(repo, clock)

	state, err := svc.GetSystemState(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.ID != 1 || state.MotorState != true {
		t.Errorf("unexpected state: %+v", state)
	}

	counts, err := svc.GetShapeCounts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(counts) != 3 {
		t.Fatalf("expected 3 shape counts, got %d", len(counts))
	}
}

func TestStateService_GetSnapshot_Success(t *testing.T) {
	clock := newMockClock(time.Now())
	userID := 1
	expectedState := &service.SystemState{
		ID:         1,
		IsPaused:   false,
		MotorState: true,
		ServoState: false,
	}
	expectedCounts := []service.ShapeCount{
		{ShapeID: 1, ShapeName: "circle", ColorLabel: "red", LiveBuffer: 2, TotalLifetime: 10},
		{ShapeID: 2, ShapeName: "triangle", ColorLabel: "green", LiveBuffer: 0, TotalLifetime: 5},
		{ShapeID: 3, ShapeName: "square", ColorLabel: "blue", LiveBuffer: 4, TotalLifetime: 15},
	}
	expectedAudits := []service.AuditRecord{
		{
			ID:        "uuid-1",
			Source:    service.AuthSourceDashboard,
			UserID:    &userID,
			Status:    service.AuditStatusSuccess,
			Timestamp: time.Now(),
		},
	}

	repo := &mockStateRepo{
		systemState:  expectedState,
		shapeCounts:  expectedCounts,
		recentAudits: expectedAudits,
	}
	svc := service.NewStateService(repo, clock)

	// Test with mqttConnected = true
	snapshot, err := svc.GetSnapshot(context.Background(), true)
	if err != nil {
		t.Fatalf("unexpected error getting snapshot: %v", err)
	}

	if snapshot.SystemState == nil || snapshot.SystemState.ID != 1 {
		t.Errorf("expected SystemState ID 1, got %+v", snapshot.SystemState)
	}
	if len(snapshot.ShapeCounts) != 3 {
		t.Errorf("expected 3 shape counts, got %d", len(snapshot.ShapeCounts))
	}
	if len(snapshot.RecentAudits) != 1 {
		t.Errorf("expected 1 recent audit, got %d", len(snapshot.RecentAudits))
	}
	if !snapshot.MQTTConnected {
		t.Errorf("expected MQTTConnected true, got %v", snapshot.MQTTConnected)
	}

	// Test with mqttConnected = false
	snapshotFalse, err := svc.GetSnapshot(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error getting snapshot: %v", err)
	}
	if snapshotFalse.MQTTConnected {
		t.Errorf("expected MQTTConnected false, got %v", snapshotFalse.MQTTConnected)
	}
}

func TestStateService_GetSnapshot_EmptySlicesNonNull(t *testing.T) {
	clock := newMockClock(time.Now())
	repo := &mockStateRepo{
		systemState:  &service.SystemState{ID: 1},
		shapeCounts:  nil,
		recentAudits: nil,
	}
	svc := service.NewStateService(repo, clock)

	snapshot, err := svc.GetSnapshot(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if snapshot.ShapeCounts == nil {
		t.Error("expected non-nil empty shape counts slice")
	}
	if snapshot.RecentAudits == nil {
		t.Error("expected non-nil empty recent audits slice")
	}
}

func TestStateService_GetSnapshot_ErrorHandling(t *testing.T) {
	clock := newMockClock(time.Now())

	t.Run("SystemState repo error", func(t *testing.T) {
		repo := &mockStateRepo{getErr: errors.New("db error")}
		svc := service.NewStateService(repo, clock)

		_, err := svc.GetSnapshot(context.Background(), true)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("RecentAudits repo error", func(t *testing.T) {
		repo := &mockStateRepo{
			systemState: &service.SystemState{ID: 1},
			shapeCounts: []service.ShapeCount{},
			auditsErr:   errors.New("audit query failed"),
		}
		svc := service.NewStateService(repo, clock)

		_, err := svc.GetSnapshot(context.Background(), true)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
