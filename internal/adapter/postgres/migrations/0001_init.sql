CREATE TABLE users (
    id               BIGSERIAL PRIMARY KEY,
    google_sub       TEXT        NOT NULL UNIQUE,
    email            TEXT        NOT NULL,
    name             TEXT        NOT NULL DEFAULT '',
    avatar_url       TEXT        NOT NULL DEFAULT '',
    telegram_chat_id TEXT        UNIQUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    token_hash BYTEA       PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);
CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE TABLE telegram_link_codes (
    code       TEXT        PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE watches (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    from_code     TEXT        NOT NULL,
    to_code       TEXT        NOT NULL,
    travel_date   DATE        NOT NULL,
    max_price     NUMERIC(14, 0) NOT NULL DEFAULT 0,
    min_seats     INT         NOT NULL DEFAULT 0,
    car_type      TEXT        NOT NULL DEFAULT '',
    active        BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_check    TIMESTAMPTZ,
    last_status   TEXT        NOT NULL DEFAULT '',
    last_matches  INT         NOT NULL DEFAULT 0,
    last_sig      TEXT        NOT NULL DEFAULT '',
    last_notified TIMESTAMPTZ,
    CHECK (from_code <> to_code)
);
CREATE INDEX watches_user_idx ON watches (user_id, created_at DESC);
CREATE INDEX watches_active_idx ON watches (from_code, to_code, travel_date) WHERE active;
