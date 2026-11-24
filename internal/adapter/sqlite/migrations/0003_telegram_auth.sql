-- migrate:no-fk
-- Google o'rniga Telegram orqali kirish: users jadvali Telegram ID bo'yicha qayta quriladi.
-- Telegram ulangan eski foydalanuvchilar (va ularning kuzatuvlari) saqlanadi; qolganlari o'chadi.
-- Barcha sessiyalar bekor qilinadi: hamma Telegram orqali qayta kiradi.

CREATE TABLE users_new (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    telegram_id   TEXT    NOT NULL UNIQUE,
    username      TEXT    NOT NULL DEFAULT '',
    name          TEXT    NOT NULL DEFAULT '',
    created_at    INTEGER NOT NULL,
    last_login_at INTEGER NOT NULL
);

INSERT INTO users_new (id, telegram_id, name, created_at, last_login_at)
     SELECT id, telegram_chat_id, name, created_at, last_login_at
       FROM users WHERE telegram_chat_id IS NOT NULL;

DELETE FROM sessions;
DELETE FROM watches WHERE user_id NOT IN (SELECT id FROM users_new);

DROP TABLE users;
ALTER TABLE users_new RENAME TO users;

DROP TABLE telegram_link_codes;

-- Brauzer ochgan kirish so'rovi. user_id botda tasdiqlangandan keyin to'ladi.
CREATE TABLE login_requests (
    code       TEXT    PRIMARY KEY,
    user_id    INTEGER REFERENCES users (id) ON DELETE CASCADE,
    expires_at INTEGER NOT NULL
);
CREATE INDEX login_requests_expires_idx ON login_requests (expires_at);
