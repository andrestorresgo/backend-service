-- 003_action_logs.sql: Table for recording system actions (servo, motor, detection, lockdown)

CREATE TABLE IF NOT EXISTS action_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    action_type VARCHAR(50) NOT NULL CHECK (action_type IN ('SERVO', 'MOTOR', 'DETECTION', 'LOCKDOWN')),
    action_name VARCHAR(50) NOT NULL,
    details TEXT NOT NULL DEFAULT '',
    source VARCHAR(50) NOT NULL DEFAULT 'SYSTEM',
    timestamp TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_action_logs_timestamp ON action_logs(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_action_logs_action_type ON action_logs(action_type);
