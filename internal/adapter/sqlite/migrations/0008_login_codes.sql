-- "Telegram'siz qurilma" uchun kod bilan kirish: botga /login yozgan foydalanuvchiga beriladigan bir martalik kodlar.
-- Kodning o'zi emas, sha256 xeshi saqlanadi. Bitta Telegram akkauntda bitta amal qiluvchi kod.
CREATE TABLE login_codes (
    code_hash   BLOB    PRIMARY KEY,
    telegram_id TEXT    NOT NULL CHECK (telegram_id <> ''),
    username    TEXT    NOT NULL DEFAULT '',
    name        TEXT    NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    expires_at  INTEGER NOT NULL
);
CREATE INDEX login_codes_telegram_idx ON login_codes (telegram_id);
CREATE INDEX login_codes_expires_idx ON login_codes (expires_at);
