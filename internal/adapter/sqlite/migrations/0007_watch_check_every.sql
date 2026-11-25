-- Admin belgilaydigan kuzatuv uchun alohida tekshiruv oralig'i (soniya). 0 = umumiy jadval bo'yicha.
ALTER TABLE watches ADD COLUMN check_every_seconds INTEGER NOT NULL DEFAULT 0 CHECK (check_every_seconds >= 0);
