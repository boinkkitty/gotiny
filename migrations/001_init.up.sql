CREATE TABLE keys (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(7) NOT NULL UNIQUE,
    status      VARCHAR(10) NOT NULL DEFAULT 'available',
    claimed_by  UUID,
    claimed_at  TIMESTAMPTZ,
    used_at     TIMESTAMPTZ
);

CREATE INDEX idx_keys_status ON keys (status) WHERE status = 'available';

CREATE TABLE urls (
    id           BIGSERIAL PRIMARY KEY,
    short_code   VARCHAR(7) NOT NULL UNIQUE,
    original_url TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
