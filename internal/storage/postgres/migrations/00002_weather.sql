-- +goose Up

-- Coordinates per subscription
ALTER TABLE subscriptions
    ADD COLUMN IF NOT EXISTS lat DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS lon DOUBLE PRECISION NOT NULL DEFAULT 0;

-- Persistent daily usage counter (guarantees limit across restarts)
CREATE TABLE IF NOT EXISTS daily_usage (
    day DATE NOT NULL,
    subscription_id uuid NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    used INT NOT NULL DEFAULT 0,
    PRIMARY KEY (day, subscription_id)
);

-- Dedup storage for sent alerts
CREATE TABLE IF NOT EXISTS sent_alerts (
    subscription_id uuid NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    fingerprint TEXT NOT NULL,
    sent_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (subscription_id, fingerprint)
);

-- +goose Down

DROP TABLE IF EXISTS sent_alerts;
DROP TABLE IF EXISTS daily_usage;

ALTER TABLE subscriptions
    DROP COLUMN IF EXISTS lon,
    DROP COLUMN IF EXISTS lat;
