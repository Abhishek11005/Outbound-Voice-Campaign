-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Citus extension is optional for local development
DO $$
BEGIN
    -- Try to create citus extension, ignore if it fails (not available)
    BEGIN
        CREATE EXTENSION IF NOT EXISTS citus;
        RAISE NOTICE 'Citus extension created successfully';
    EXCEPTION
        WHEN OTHERS THEN
            RAISE NOTICE 'Citus extension not available (this is normal for local development)';
    END;
END $$;

CREATE TABLE IF NOT EXISTS campaigns (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    time_zone TEXT NOT NULL,
    max_concurrent_calls INTEGER NOT NULL CHECK (max_concurrent_calls > 0),
    status TEXT NOT NULL,
    retry_max_attempts INTEGER NOT NULL CHECK (retry_max_attempts >= 0),
    retry_base_delay_ms BIGINT NOT NULL,
    retry_max_delay_ms BIGINT NOT NULL,
    retry_jitter DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_campaigns_name ON campaigns (LOWER(name));

CREATE TABLE IF NOT EXISTS campaign_business_hours (
    id BIGSERIAL PRIMARY KEY,
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    day_of_week SMALLINT NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    start_minute SMALLINT NOT NULL CHECK (start_minute BETWEEN 0 AND 1439),
    end_minute SMALLINT NOT NULL CHECK (end_minute BETWEEN 0 AND 1439 AND end_minute > start_minute),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (campaign_id, day_of_week, start_minute, end_minute)
);

CREATE TABLE IF NOT EXISTS campaign_targets (
    id UUID PRIMARY KEY,
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    phone_number TEXT NOT NULL,
    payload JSONB,
    state TEXT NOT NULL DEFAULT 'pending',
    scheduled_at TIMESTAMPTZ,
    last_attempt_at TIMESTAMPTZ,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_campaign_targets_campaign_state ON campaign_targets (campaign_id, state, scheduled_at);
CREATE INDEX IF NOT EXISTS idx_campaign_targets_phone ON campaign_targets (phone_number);

CREATE TABLE IF NOT EXISTS campaign_statistics (
    campaign_id UUID PRIMARY KEY REFERENCES campaigns(id) ON DELETE CASCADE,
    total_calls BIGINT NOT NULL DEFAULT 0,
    completed_calls BIGINT NOT NULL DEFAULT 0,
    failed_calls BIGINT NOT NULL DEFAULT 0,
    in_progress_calls BIGINT NOT NULL DEFAULT 0,
    pending_calls BIGINT NOT NULL DEFAULT 0,
    retries_attempted BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS calls (
    id UUID PRIMARY KEY,
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    phone_number TEXT NOT NULL,
    status TEXT NOT NULL,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMPTZ,
    scheduled_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error TEXT
);

CREATE INDEX IF NOT EXISTS idx_calls_campaign_status ON calls (campaign_id, status, scheduled_at);

CREATE TABLE IF NOT EXISTS campaign_events (
    id BIGSERIAL PRIMARY KEY,
    campaign_id UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE OR REPLACE FUNCTION set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Drop existing triggers if they exist, then recreate them
DROP TRIGGER IF EXISTS trg_campaigns_updated ON campaigns;
CREATE TRIGGER trg_campaigns_updated
BEFORE UPDATE ON campaigns
FOR EACH ROW EXECUTE FUNCTION set_timestamp();

DROP TRIGGER IF EXISTS trg_campaign_business_hours_updated ON campaign_business_hours;
CREATE TRIGGER trg_campaign_business_hours_updated
BEFORE UPDATE ON campaign_business_hours
FOR EACH ROW EXECUTE FUNCTION set_timestamp();

DROP TRIGGER IF EXISTS trg_calls_updated ON calls;
CREATE TRIGGER trg_calls_updated
BEFORE UPDATE ON calls
FOR EACH ROW EXECUTE FUNCTION set_timestamp();

DROP TRIGGER IF EXISTS trg_campaign_targets_updated ON campaign_targets;
CREATE TRIGGER trg_campaign_targets_updated
BEFORE UPDATE ON campaign_targets
FOR EACH ROW EXECUTE FUNCTION set_timestamp();

DROP TRIGGER IF EXISTS trg_campaign_statistics_updated ON campaign_statistics;
CREATE TRIGGER trg_campaign_statistics_updated
BEFORE UPDATE ON campaign_statistics
FOR EACH ROW EXECUTE FUNCTION set_timestamp();

-- Conditionally create distributed tables if Citus is available
DO $$
BEGIN
    -- Check if citus extension exists and create distributed tables
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'citus') THEN
        -- Distribute tables for horizontal scaling
        PERFORM create_distributed_table('campaigns', 'id');
        PERFORM create_distributed_table('calls', 'campaign_id');
        PERFORM create_distributed_table('campaign_targets', 'campaign_id');
        PERFORM create_reference_table('campaign_business_hours');
        PERFORM create_reference_table('campaign_statistics');
        PERFORM create_distributed_table('campaign_events', 'campaign_id');
        RAISE NOTICE 'Tables distributed with Citus';
    ELSE
        RAISE NOTICE 'Citus not available, skipping table distribution (normal for local development)';
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Clean up Citus distribution if it exists
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'citus') THEN
        -- Undistribute tables before dropping (if Citus is available)
        BEGIN
            PERFORM undistribute_table('campaign_events');
            PERFORM undistribute_table('campaign_targets');
            PERFORM undistribute_table('calls');
            PERFORM undistribute_table('campaigns');
            RAISE NOTICE 'Tables undistributed from Citus';
        EXCEPTION
            WHEN OTHERS THEN
                RAISE NOTICE 'Some tables may not have been distributed, continuing cleanup';
        END;
    END IF;
END $$;

DROP TRIGGER IF EXISTS trg_campaign_statistics_updated ON campaign_statistics;
DROP TRIGGER IF EXISTS trg_campaign_targets_updated ON campaign_targets;
DROP TRIGGER IF EXISTS trg_calls_updated ON calls;
DROP TRIGGER IF EXISTS trg_campaign_business_hours_updated ON campaign_business_hours;
DROP TRIGGER IF EXISTS trg_campaigns_updated ON campaigns;
DROP FUNCTION IF EXISTS set_timestamp();
DROP TABLE IF EXISTS campaign_events;
DROP TABLE IF EXISTS campaign_statistics;
DROP TABLE IF EXISTS campaign_targets;
DROP TABLE IF EXISTS calls;
DROP TABLE IF EXISTS campaign_business_hours;
DROP TABLE IF EXISTS campaigns;
-- +goose StatementEnd
