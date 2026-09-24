-- 002_motor_speed_states.sql: Migrate motor_state from BOOLEAN to VARCHAR(10) with 3 states (OFF, ON, MEDIUM)

ALTER TABLE system_state 
    ALTER COLUMN motor_state DROP DEFAULT,
    ALTER COLUMN motor_state TYPE VARCHAR(10) USING CASE 
        WHEN motor_state = TRUE THEN 'ON' 
        ELSE 'OFF' 
    END,
    ALTER COLUMN motor_state SET DEFAULT 'OFF';
