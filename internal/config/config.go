// Package config — muhit o'zgaruvchilaridan sozlamalarni o'qish.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// MinPollInterval — saytga ortiqcha yuklama bermaslik uchun eng kichik oraliq.
const MinPollInterval = 2 * time.Minute

type Config struct {
	Listen   string
	BaseURL  string // tashqi manzil: https://bilet.example.uz (OAuth redirect shundan yasaladi)
	TimeZone string
	DataDir  string

	DatabaseURL string

	GoogleClientID     string
	GoogleClientSecret string

	TelegramToken string

	RailwayBaseURL string
	RailwayLang    string
	RailwayCookie  string
	StationsFile   string

	PollInterval        time.Duration
	MaxSearchesPerCycle int
	MaxWatchesPerUser   int
	SessionTTL          time.Duration
	AdminEmails         []string

	Warnings []string
}

func (c Config) SecureCookies() bool { return strings.HasPrefix(c.BaseURL, "https://") }

func (c Config) GoogleRedirectURL() string { return c.BaseURL + "/auth/google/callback" }

func Load() (Config, error) {
	c := Config{
		Listen:             env("LISTEN_ADDR", ":8080"),
		BaseURL:            strings.TrimRight(env("BASE_URL", "http://localhost:8080"), "/"),
		TimeZone:           env("TZ_NAME", "Asia/Tashkent"),
		DataDir:            env("DATA_DIR", "./data"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		TelegramToken:      os.Getenv("TELEGRAM_BOT_TOKEN"),
		RailwayBaseURL:     strings.TrimRight(env("RAILWAY_BASE", "https://eticket.railway.uz"), "/"),
		RailwayLang:        env("RAILWAY_LANG", "uz"),
		RailwayCookie:      os.Getenv("RAILWAY_COOKIE"),
		StationsFile:       os.Getenv("STATIONS_FILE"),
	}

	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL berilmagan"))
	}
	if c.GoogleClientID == "" || c.GoogleClientSecret == "" {
		errs = append(errs, errors.New("GOOGLE_CLIENT_ID va GOOGLE_CLIENT_SECRET kerak"))
	}

	var err error
	if c.PollInterval, err = duration("POLL_INTERVAL", "5m"); err != nil {
		errs = append(errs, err)
	} else if c.PollInterval < MinPollInterval {
		c.Warnings = append(c.Warnings, fmt.Sprintf("POLL_INTERVAL %s juda kichik, %s ga oshirildi", c.PollInterval, MinPollInterval))
		c.PollInterval = MinPollInterval
	}
	if c.SessionTTL, err = duration("SESSION_TTL", "720h"); err != nil {
		errs = append(errs, err)
	}
	if c.MaxSearchesPerCycle, err = integer("MAX_SEARCHES_PER_CYCLE", 30); err != nil {
		errs = append(errs, err)
	}
	if c.MaxWatchesPerUser, err = integer("MAX_WATCHES_PER_USER", 10); err != nil {
		errs = append(errs, err)
	}
	for _, e := range strings.Split(os.Getenv("ADMIN_EMAILS"), ",") {
		if e = strings.TrimSpace(e); e != "" {
			c.AdminEmails = append(c.AdminEmails, e)
		}
	}

	if c.TelegramToken == "" {
		c.Warnings = append(c.Warnings, "TELEGRAM_BOT_TOKEN berilmagan — xabarlar yuborilmaydi, Telegram ulash ishlamaydi")
	}
	if !c.SecureCookies() {
		c.Warnings = append(c.Warnings, "BASE_URL https emas — productionda albatta https ishlating")
	}
	return c, errors.Join(errs...)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func duration(k, def string) (time.Duration, error) {
	d, err := time.ParseDuration(env(k, def))
	if err != nil {
		return 0, fmt.Errorf("%s noto'g'ri: %w", k, err)
	}
	return d, nil
}

func integer(k string, def int) (int, error) {
	v := os.Getenv(k)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s noto'g'ri", k)
	}
	return n, nil
}
