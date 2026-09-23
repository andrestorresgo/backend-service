package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	MotorState      bool       `json:"motor_state"`
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
	IsPaused   bool `json:"is_paused"`
	MotorState bool `json:"motor_state"`
	ServoState bool `json:"servo_state"`
	RedCount   int  `json:"red_count"`
	GreenCount int  `json:"green_count"`
	BlueCount  int  `json:"blue_count"`
}

// RolloverData contains decoded batch rollover fields from factory/rollover.
type RolloverData struct {
	ShapeID   int    `json:"shape_id"`
	ShapeName string `json:"shape_name"`
	Timestamp int64  `json:"timestamp"`
}

// StateRepository abstracts persistence operations for system state and counters.
type StateRepository interface {
	UpdateTelemetry(ctx context.Context, state SystemState, counts map[int]int, updatedAt time.Time) error
	IncrementRollover(ctx context.Context, shapeID int, increment int, updatedAt time.Time) error
	GetSystemState(ctx context.Context) (*SystemState, error)
	GetShapeCounts(ctx context.Context) ([]ShapeCount, error)
}

// StateService coordinates authoritative telemetry synchronization and atomic rollover accumulation.
type StateService struct {
	repo  StateRepository
	clock Clock
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
