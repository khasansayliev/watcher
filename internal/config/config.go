// Package config — muhit o'zgaruvchilaridan sozlamalarni o'qish.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// MinPollInterval — saytga ortiqcha yuklama bermaslik uchun eng kichik oraliq.
const MinPollInterval = 2 * time.Minute

type Config struct {
	Listen          string
	DataDir         string
	TimeZone        string
	PollInterval    time.Duration
	ManualSearchGap time.Duration

	TelegramToken string
	DefaultChatID string

	RailwayBaseURL string
	RailwayLang    string
	RailwayCookie  string
	StationsFile   string

	UIUser     string
	UIPassword string

	Warnings []string
}

func Load() (Config, error) {
	c := Config{
		Listen:          env("LISTEN_ADDR", ":8080"),
		DataDir:         env("DATA_DIR", "./data"),
		TimeZone:        env("TZ_NAME", "Asia/Tashkent"),
		ManualSearchGap: 10 * time.Second,
		TelegramToken:   os.Getenv("TELEGRAM_BOT_TOKEN"),
		DefaultChatID:   os.Getenv("TELEGRAM_DEFAULT_CHAT_ID"),
		RailwayBaseURL:  strings.TrimRight(env("RAILWAY_BASE", "https://eticket.railway.uz"), "/"),
		RailwayLang:     env("RAILWAY_LANG", "uz"),
		RailwayCookie:   os.Getenv("RAILWAY_COOKIE"),
		StationsFile:    os.Getenv("STATIONS_FILE"),
		UIUser:          env("UI_USER", "admin"),
		UIPassword:      os.Getenv("UI_PASSWORD"),
	}

	d, err := time.ParseDuration(env("POLL_INTERVAL", "5m"))
	if err != nil {
		return c, fmt.Errorf("POLL_INTERVAL noto'g'ri: %w", err)
	}
	if d < MinPollInterval {
		c.Warnings = append(c.Warnings, fmt.Sprintf("POLL_INTERVAL %s juda kichik, %s ga oshirildi", d, MinPollInterval))
		d = MinPollInterval
	}
	c.PollInterval = d

	if c.TelegramToken == "" {
		c.Warnings = append(c.Warnings, "TELEGRAM_BOT_TOKEN berilmagan — xabarlar yuborilmaydi")
	}
	if c.UIPassword == "" {
		c.Warnings = append(c.Warnings, "UI_PASSWORD berilmagan — UI parolsiz ochiq")
	}
	return c, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
