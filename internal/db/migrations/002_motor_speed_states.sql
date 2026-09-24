-- 002_motor_speed_states.sql: Migrate motor_state from BOOLEAN to VARCHAR(10) with 3 states (OFF, ON, MEDIUM)

DO $$ 
BEGIN 
    IF EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name = 'system_state' 
          AND column_name = 'motor_state' 
          AND data_type = 'boolean'
    ) THEN 
        EXECUTE 'ALTER TABLE system_state 
            ALTER COLUMN motor_state DROP DEFAULT,
            ALTER COLUMN motor_state TYPE VARCHAR(10) USING CASE 
                WHEN motor_state = TRUE THEN ''ON'' 
                ELSE ''OFF'' 
            END,
            ALTER COLUMN motor_state SET DEFAULT ''OFF''';
    END IF;
END $$;

UPDATE system_state
SET motor_state = CASE 
    WHEN UPPER(motor_state) IN ('TRUE', '1', 'ON') THEN 'ON'
    WHEN UPPER(motor_state) IN ('MEDIUM', '2') THEN 'MEDIUM'
    ELSE 'OFF'
END
WHERE motor_state NOT IN ('ON', 'MEDIUM', 'OFF');
