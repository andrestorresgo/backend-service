-- 001_init.sql: Initial schema and seed data

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Users table storing plaintext credentials per system specification
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY,
    username VARCHAR(50) NOT NULL,
    pin VARCHAR(10) NOT NULL,
    failed_attempts INTEGER NOT NULL DEFAULT 0,
    locked_until TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Enforced singleton table tracking authoritative physical hardware telemetry
CREATE TABLE IF NOT EXISTS system_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    is_paused BOOLEAN NOT NULL DEFAULT FALSE,
    motor_state BOOLEAN NOT NULL DEFAULT FALSE,
    servo_state BOOLEAN NOT NULL DEFAULT FALSE,
    last_telemetry_at TIMESTAMPTZ NULL
);

-- Shape counters tracking live buffer on Board B and accumulated lifetime counts
CREATE TABLE IF NOT EXISTS shape_counts (
    shape_id INTEGER PRIMARY KEY,
    shape_name VARCHAR(20) NOT NULL,
    color_label VARCHAR(20) NOT NULL,
    live_buffer INTEGER NOT NULL DEFAULT 0 CHECK (live_buffer >= 0 AND live_buffer <= 5),
    total_lifetime BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Append-only access audit log tracking all authentication attempts
CREATE TABLE IF NOT EXISTS auth_audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source VARCHAR(20) NOT NULL CHECK (source IN ('KEYPAD', 'DASHBOARD')),
    user_id INTEGER NULL REFERENCES users(id) ON DELETE SET NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('SUCCESS', 'INVALID_PIN', 'USER_LOCKED', 'USER_NOT_FOUND')),
    timestamp TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Seed default operators
INSERT INTO users (id, username, pin, failed_attempts, locked_until)
VALUES
    (1, 'Andres', '1234', 0, NULL),
    (2, 'Aldo', '5678', 0, NULL)
ON CONFLICT (id) DO NOTHING;

-- Seed singleton system state
INSERT INTO system_state (id, is_paused, motor_state, servo_state, last_telemetry_at)
VALUES
    (1, FALSE, FALSE, FALSE, NULL)
ON CONFLICT (id) DO NOTHING;

-- Seed default shape counters
INSERT INTO shape_counts (shape_id, shape_name, color_label, live_buffer, total_lifetime)
VALUES
    (1, 'circle', 'red', 0, 0),
    (2, 'triangle', 'green', 0, 0),
    (3, 'square', 'blue', 0, 0)
ON CONFLICT (shape_id) DO NOTHING;
