-- Har bir kuzatuv uchun har bir tekshiruv so'rovining yozuvi. req_time — Unix millisekund (UTC).
CREATE TABLE watch_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    watch_id    INTEGER NOT NULL REFERENCES watches (id) ON DELETE CASCADE,
    req_time    INTEGER NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    ok          INTEGER NOT NULL CHECK (ok IN (0, 1)),
    error       TEXT    NOT NULL DEFAULT '',
    train_count INTEGER NOT NULL DEFAULT 0,
    match_count INTEGER NOT NULL DEFAULT 0,
    notified    INTEGER NOT NULL DEFAULT 0 CHECK (notified IN (0, 1)),
    notify_err  TEXT    NOT NULL DEFAULT '',
    manual      INTEGER NOT NULL DEFAULT 0 CHECK (manual IN (0, 1)),
    trains      TEXT    NOT NULL DEFAULT '[]'
);
-- Ro'yxat: watch_id bo'yicha req_time kamayish tartibida (pagination).
CREATE INDEX watch_logs_watch_time_idx ON watch_logs (watch_id, req_time DESC, id DESC);
-- Eskirgan yozuvlarni tozalash uchun.
CREATE INDEX watch_logs_time_idx ON watch_logs (req_time);
