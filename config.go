package main

import (
	"log"
	"os"
	"strings"
	"time"
)

// Saytga ortiqcha yuklama bermaslik uchun eng kichik ruxsat etilgan oraliq.
const minPollInterval = 2 * time.Minute

type Config struct {
	Listen        string
	DataDir       string
	BotToken      string
	DefaultChatID string
	RailwayBase   string
	RailwayCookie string
	Lang          string
	StationsFile  string
	UIUser        string
	UIPassword    string
	PollInterval  time.Duration
}

func loadConfig() Config {
	c := Config{
		Listen:        env("LISTEN_ADDR", ":8080"),
		DataDir:       env("DATA_DIR", "./data"),
		BotToken:      os.Getenv("TELEGRAM_BOT_TOKEN"),
		DefaultChatID: os.Getenv("TELEGRAM_DEFAULT_CHAT_ID"),
		RailwayBase:   strings.TrimRight(env("RAILWAY_BASE", "https://eticket.railway.uz"), "/"),
		RailwayCookie: os.Getenv("RAILWAY_COOKIE"),
		Lang:          env("RAILWAY_LANG", "uz"),
		StationsFile:  os.Getenv("STATIONS_FILE"),
		UIUser:        env("UI_USER", "admin"),
		UIPassword:    os.Getenv("UI_PASSWORD"),
	}

	d, err := time.ParseDuration(env("POLL_INTERVAL", "5m"))
	if err != nil {
		log.Fatalf("POLL_INTERVAL noto'g'ri: %v", err)
	}
	if d < minPollInterval {
		log.Printf("POLL_INTERVAL %s juda kichik, %s ga oshirildi", d, minPollInterval)
		d = minPollInterval
	}
	c.PollInterval = d

	if c.BotToken == "" {
		log.Println("OGOHLANTIRISH: TELEGRAM_BOT_TOKEN berilmagan — xabarlar yuborilmaydi")
	}
	if c.UIPassword == "" {
		log.Println("OGOHLANTIRISH: UI_PASSWORD berilmagan — UI parolsiz ochiq")
	}
	return c
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
