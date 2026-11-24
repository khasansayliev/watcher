-- Kun ichida jo'nash vaqti oralig'i ("HH:MM"). Bo'sh = cheklov yo'q.
ALTER TABLE watches ADD COLUMN time_from TEXT NOT NULL DEFAULT '';
ALTER TABLE watches ADD COLUMN time_to   TEXT NOT NULL DEFAULT '';
