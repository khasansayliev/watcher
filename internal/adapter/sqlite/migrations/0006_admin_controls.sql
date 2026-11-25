-- Admin boshqaruvi: kuzatuv yaratish ruxsati, foydalanuvchi va kuzatuvni o'chirish, platforma sozlamalari.
ALTER TABLE users ADD COLUMN can_create INTEGER NOT NULL DEFAULT 0 CHECK (can_create IN (0, 1));
ALTER TABLE users ADD COLUMN disabled   INTEGER NOT NULL DEFAULT 0 CHECK (disabled IN (0, 1));
-- Hozirgi foydalanuvchilar ruxsatni saqlab qoladi; yangilari (DEFAULT 0) admin ruxsatini kutadi.
UPDATE users SET can_create = 1;

ALTER TABLE watches ADD COLUMN admin_disabled INTEGER NOT NULL DEFAULT 0 CHECK (admin_disabled IN (0, 1));

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
INSERT INTO settings (key, value) VALUES ('creation_mode', 'approved');
