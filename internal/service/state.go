package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Shape ID constants matching the hardware protocol and database seed.
const (
	ShapeCircleID   = 1
	ShapeTriangleID = 2
	ShapeSquareID   = 3

	MinLiveBuffer          = 0
	MaxLiveBuffer          = 5
	RolloverBatchIncrement = 5
)

var (
	// ErrInvalidShapeID indicates an unknown or unmapped shape identifier.
	ErrInvalidShapeID = errors.New("invalid shape id")
	// ErrShapeNotFound indicates a shape row was not found in the database.
	ErrShapeNotFound = errors.New("shape not found")
)

// SystemState represents the authoritative singleton record from Board A telemetry.
type SystemState struct {
	ID              int        `json:"id"`
	IsPaused        bool       `json:"is_paused"`
	MotorState      string     `json:"motor_state"`
	ServoState      bool       `json:"servo_state"`
	LastTelemetryAt *time.Time `json:"last_telemetry_at"`
}

// ShapeCount represents a single shape counter record in the database.
type ShapeCount struct {
	ShapeID       int       `json:"shape_id"`
	ShapeName     string    `json:"shape_name"`
	ColorLabel    string    `json:"color_label"`
	LiveBuffer    int       `json:"live_buffer"`
	TotalLifetime int64     `json:"total_lifetime"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TelemetryData contains decoded telemetry fields from factory/telemetry.
type TelemetryData struct {
	IsPaused   bool   `json:"is_paused"`
	MotorState string `json:"motor_state"`
	ServoState bool   `json:"servo_state"`
	RedCount   int    `json:"red_count"`
	GreenCount int    `json:"green_count"`
	BlueCount  int    `json:"blue_count"`
}

// RolloverData contains decoded batch rollover fields from factory/rollover.
type RolloverData struct {
	ShapeID   int    `json:"shape_id"`
	ShapeName string `json:"shape_name"`
	Timestamp int64  `json:"timestamp"`
}

// AuditRecord represents a recorded authentication audit log entry for the state snapshot.
type AuditRecord struct {
	ID        string      `json:"id"`
	Source    AuthSource  `json:"source"`
	UserID    *int        `json:"user_id"`
	Status    AuditStatus `json:"status"`
	Timestamp time.Time   `json:"timestamp"`
}

// StateSnapshot represents the consolidated system snapshot for dashboard initialization.
type StateSnapshot struct {
	SystemState   *SystemState   `json:"system_state"`
	ShapeCounts   []ShapeCount   `json:"shape_counts"`
	RecentAudits  []AuditRecord  `json:"recent_audits"`
	RecentActions []ActionRecord `json:"recent_actions"`
	MQTTConnected bool           `json:"mqtt_connected"`
}

// StateRepository abstracts persistence operations for system state, counters, and action logs.
type StateRepository interface {
	UpdateTelemetry(ctx context.Context, state SystemState, counts map[int]int, updatedAt time.Time) error
	IncrementRollover(ctx context.Context, shapeID int, increment int, updatedAt time.Time) error
	GetSystemState(ctx context.Context) (*SystemState, error)
	GetShapeCounts(ctx context.Context) ([]ShapeCount, error)
	GetRecentAudits(ctx context.Context, limit int) ([]AuditRecord, error)
	InsertActionLog(ctx context.Context, actionType ActionType, actionName string, details string, source string, timestamp time.Time) error
	GetRecentActions(ctx context.Context, limit int) ([]ActionRecord, error)
}

// StateService coordinates authoritative telemetry synchronization, atomic rollover accumulation, and lockdown action logging.
type StateService struct {
	repo          StateRepository
	clock         Clock
	publisher     ActionPublisher
	mu            sync.Mutex
	hasLastPaused bool
	lastPaused    bool
}

// NewStateService constructs a new StateService.
func NewStateService(repo StateRepository, clock Clock) *StateService {
	if clock == nil {
		clock = RealClock{}
	}
	return &StateService{
		repo:  repo,
		clock: clock,
	}
}

// SetActionPublisher configures an optional MQTT publisher for broadcasting action events.
func (s *StateService) SetActionPublisher(pub ActionPublisher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publisher = pub
}

// UpdateTelemetry updates the singleton system_state and synchronizes live_buffer counters.
func (s *StateService) UpdateTelemetry(ctx context.Context, data TelemetryData) error {
	now := s.clock.Now()

	state := SystemState{
		ID:              1,
		IsPaused:        data.IsPaused,
		MotorState:      data.MotorState,
		ServoState:      data.ServoState,
		LastTelemetryAt: &now,
	}

	counts := map[int]int{
		ShapeCircleID:   clampBuffer(data.RedCount),
		ShapeTriangleID: clampBuffer(data.GreenCount),
		ShapeSquareID:   clampBuffer(data.BlueCount),
	}

	if err := s.repo.UpdateTelemetry(ctx, state, counts, now); err != nil {
		return fmt.Errorf("failed to update telemetry in repository: %w", err)
	}

	// Detect machine pause / hardware lockout state transitions
	s.mu.Lock()
	if s.hasLastPaused && s.lastPaused != data.IsPaused {
		if data.IsPaused {
			BroadcastAction(ctx, s.repo, s.publisher, ActionTypeLockdown, "LOCKDOWN_ENGAGED", "Machine Pause engaged - Total physical actuation lockout active", "HARDWARE", now)
		} else {
			BroadcastAction(ctx, s.repo, s.publisher, ActionTypeLockdown, "LOCKDOWN_RELEASED", "Machine Pause released - Physical actuation resumed", "HARDWARE", now)
		}
	}
	s.hasLastPaused = true
	s.lastPaused = data.IsPaused
	s.mu.Unlock()

	return nil
}

// ProcessRollover atomically increments the total lifetime counter for a completed batch and resets live buffer.
func (s *StateService) ProcessRollover(ctx context.Context, data RolloverData) error {
	shapeID := data.ShapeID
	if shapeID == 0 {
		shapeID = resolveShapeIDByName(data.ShapeName)
	}

	if shapeID < ShapeCircleID || shapeID > ShapeSquareID {
		return fmt.Errorf("%w: %d", ErrInvalidShapeID, shapeID)
	}

	now := s.clock.Now()
	if err := s.repo.IncrementRollover(ctx, shapeID, RolloverBatchIncrement, now); err != nil {
		return fmt.Errorf("failed to process rollover in repository: %w", err)
	}

	BroadcastAction(ctx, s.repo, s.publisher, ActionTypeDetection, "BATCH_ROLLOVER", fmt.Sprintf("Batch complete for shape %s (Shape ID %d) - +5 accumulated", data.ShapeName, shapeID), "HARDWARE", now)

	return nil
}

// GetSystemState returns the authoritative system state.
func (s *StateService) GetSystemState(ctx context.Context) (*SystemState, error) {
	return s.repo.GetSystemState(ctx)
}

// GetShapeCounts returns all shape counters.
func (s *StateService) GetShapeCounts(ctx context.Context) ([]ShapeCount, error) {
	return s.repo.GetShapeCounts(ctx)
}

// GetRecentActions retrieves recent system action records.
func (s *StateService) GetRecentActions(ctx context.Context, limit int) ([]ActionRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	records, err := s.repo.GetRecentActions(ctx, limit)
	if err != nil {
		return nil, err
	}
	if records == nil {
		records = []ActionRecord{}
	}
	return records, nil
}

// GetSnapshot retrieves the consolidated system state, shape counters, recent auth audits, and recent system actions.
func (s *StateService) GetSnapshot(ctx context.Context, mqttConnected bool) (StateSnapshot, error) {
	systemState, err := s.repo.GetSystemState(ctx)
	if err != nil {
		return StateSnapshot{}, fmt.Errorf("failed to get system state: %w", err)
	}

	shapeCounts, err := s.repo.GetShapeCounts(ctx)
	if err != nil {
		return StateSnapshot{}, fmt.Errorf("failed to get shape counts: %w", err)
	}
	if shapeCounts == nil {
		shapeCounts = []ShapeCount{}
	}

	recentAudits, err := s.repo.GetRecentAudits(ctx, 10)
	if err != nil {
		return StateSnapshot{}, fmt.Errorf("failed to get recent audits: %w", err)
	}
	if recentAudits == nil {
		recentAudits = []AuditRecord{}
	}

	recentActions, err := s.repo.GetRecentActions(ctx, 20)
	if err != nil {
		return StateSnapshot{}, fmt.Errorf("failed to get recent actions: %w", err)
	}
	if recentActions == nil {
		recentActions = []ActionRecord{}
	}

	return StateSnapshot{
		SystemState:   systemState,
		ShapeCounts:   shapeCounts,
		RecentAudits:  recentAudits,
		RecentActions: recentActions,
		MQTTConnected: mqttConnected,
	}, nil
}

// clampBuffer ensures live buffer counts satisfy the CHECK (live_buffer >= 0 AND live_buffer <= 5) constraint.
func clampBuffer(val int) int {
	if val < MinLiveBuffer {
		return MinLiveBuffer
	}
	if val > MaxLiveBuffer {
		return MaxLiveBuffer
	}
	return val
}

// resolveShapeIDByName attempts to resolve a shape name or color label to its numeric ID.
func resolveShapeIDByName(name string) int {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "circle", "red":
		return ShapeCircleID
	case "triangle", "green":
		return ShapeTriangleID
	case "square", "blue":
		return ShapeSquareID
	default:
		return 0
	}
}
