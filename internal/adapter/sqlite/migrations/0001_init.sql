-- Vaqtlar INTEGER: Unix millisekund (UTC). NULL = hali bo'lmagan.
-- Sanalar TEXT: 'YYYY-MM-DD' (leksikografik solishtirish to'g'ri ishlaydi).

-- Foydalanuvchi Telegram orqali kiradi; telegram_id — bot bilan shaxsiy chat ID'si ham.
CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    telegram_id   INTEGER NOT NULL UNIQUE CHECK (telegram_id > 0),
    username      TEXT    NOT NULL DEFAULT '',
    name          TEXT    NOT NULL DEFAULT '',
    photo_url     TEXT    NOT NULL DEFAULT '',
    created_at    INTEGER NOT NULL,
    last_login_at INTEGER NOT NULL
);

CREATE TABLE sessions (
    token_hash BLOB    PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);
CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE TABLE watches (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id       INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    from_code     TEXT    NOT NULL,
    to_code       TEXT    NOT NULL,
    travel_date   TEXT    NOT NULL CHECK (travel_date GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    max_price     REAL    NOT NULL DEFAULT 0,
    min_seats     INTEGER NOT NULL DEFAULT 0,
    car_type      TEXT    NOT NULL DEFAULT '',
    active        INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at    INTEGER NOT NULL,
    last_check    INTEGER,
    last_status   TEXT    NOT NULL DEFAULT '',
    last_matches  INTEGER NOT NULL DEFAULT 0,
    last_sig      TEXT    NOT NULL DEFAULT '',
    last_notified INTEGER,
    CHECK (from_code <> to_code)
);
CREATE INDEX watches_user_idx ON watches (user_id, created_at DESC);
CREATE INDEX watches_active_idx ON watches (from_code, to_code, travel_date) WHERE active = 1;
