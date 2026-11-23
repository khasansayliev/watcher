//go:build !nodriver

package main

// SQLite drayveri (sof Go, cgo kerak emas). Faqat shu fayl tashqi kutubxonaga bog'liq;
// qolgan kod standart database/sql bilan ishlaydi.
import _ "modernc.org/sqlite"

const dbDriver = "sqlite"
