-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS subscriptions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
    owner_ref text NOT NULL UNIQUE,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_subscriptions_owner_ref ON subscriptions (owner_ref);

CREATE INDEX IF NOT EXISTS idx_subscriptions_active ON subscriptions (active);

CREATE TABLE IF NOT EXISTS endpoints (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
    kind text NOT NULL,
    address text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (kind, address)
);

-- One source(endpoint) -> one scheduler(subscription). A subscription may have multiple endpoints in future,
-- but each endpoint belongs to only one subscription.
CREATE TABLE IF NOT EXISTS subscription_endpoints (
    subscription_id uuid NOT NULL REFERENCES subscriptions (id) ON DELETE CASCADE,
    endpoint_id uuid NOT NULL REFERENCES endpoints (id) ON DELETE CASCADE,
    PRIMARY KEY (subscription_id, endpoint_id),
    UNIQUE (endpoint_id)
);

CREATE INDEX IF NOT EXISTS idx_sub_endpoints_sub ON subscription_endpoints (subscription_id);

CREATE TABLE IF NOT EXISTS schedules (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid (),
    subscription_id uuid NOT NULL REFERENCES subscriptions (id) ON DELETE CASCADE,
    kind text NOT NULL,
    expr text NOT NULL,
    tz text NOT NULL DEFAULT 'Europe/Vilnius',
    starts_at timestamptz,
    ends_at timestamptz,
    next_run_at timestamptz,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_schedules_next_run_at ON schedules (next_run_at);

CREATE INDEX IF NOT EXISTS idx_schedules_subscription_id ON schedules (subscription_id);

CREATE INDEX IF NOT EXISTS idx_schedules_active ON schedules (active);

CREATE TABLE IF NOT EXISTS runs (
    id bigserial PRIMARY KEY,
    subscription_id uuid NOT NULL REFERENCES subscriptions (id) ON DELETE CASCADE,
    scheduled_for timestamptz NOT NULL,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    status text NOT NULL,
    payload text,
    error text
);

CREATE INDEX IF NOT EXISTS idx_runs_subscription_id ON runs (subscription_id);

CREATE INDEX IF NOT EXISTS idx_runs_started_at ON runs (started_at);

-- +goose Down
DROP TABLE IF EXISTS runs;

DROP TABLE IF EXISTS schedules;

DROP TABLE IF EXISTS subscription_endpoints;

DROP TABLE IF EXISTS endpoints;

DROP TABLE IF EXISTS subscriptions;