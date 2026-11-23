-- Tekshiruv oraliqlari jadvali: jo'nashga qolgan kunga qarab.
-- max_days_left = 0 — "qolganlari" (hech bir chegaraga tushmaganlar).
CREATE TABLE polling_rules (
    max_days_left    INTEGER PRIMARY KEY CHECK (max_days_left BETWEEN 0 AND 365),
    interval_seconds INTEGER NOT NULL CHECK (interval_seconds BETWEEN 10 AND 604800),
    updated_at       INTEGER NOT NULL DEFAULT 0,
    updated_by       TEXT    NOT NULL DEFAULT ''
);

INSERT INTO polling_rules (max_days_left, interval_seconds) VALUES
    (2, 10),        -- 2 kundan kam: 10 soniya
    (10, 300),      -- 10 kundan kam: 5 daqiqa
    (30, 3600),     -- 30 kundan kam: 1 soat
    (0, 43200);     -- qolganlari: 12 soat
