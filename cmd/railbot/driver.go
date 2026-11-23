//go:build !nodriver

package main

// PostgreSQL drayveri. Faqat shu fayl tashqi kutubxonaga bog'liq;
// qolgan kod standart database/sql bilan ishlaydi.
import _ "github.com/jackc/pgx/v5/stdlib"

const dbDriver = "pgx"
