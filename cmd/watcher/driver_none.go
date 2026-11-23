//go:build nodriver

package main

// `-tags nodriver` — drayversiz kompilyatsiya (masalan, modul proksisi yo'q muhitda tekshirish uchun).
// Bunday binar bazaga ulana olmaydi.
const dbDriver = "pgx"
