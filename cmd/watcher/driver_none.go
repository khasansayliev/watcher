//go:build nodriver

package main

// `-tags nodriver` — drayversiz kompilyatsiya (masalan, modul proksisi yo'q muhitda tekshirish uchun).
// Bunday binar bazani ocha olmaydi.
const dbDriver = "sqlite"
