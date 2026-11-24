// Package config — muhit o'zgaruvchilaridan sozlamalarni o'qish.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Listen   string
	BaseURL  string // tashqi manzil: https://bilet.example.uz (cookie'lar Secure bo'lishi shu bilan aniqlanadi)
	TimeZone string
	DataDir  string

	DatabasePath string // SQLite fayli

	TelegramToken string

	RailwayBaseURL string
	RailwayLang    string
	RailwayCookie  string
	StationsFile   string

	MaxSearchesPerCycle int
	MaxWatchesPerUser   int
	SessionTTL          time.Duration
<<<<<<< Updated upstream
	AdminTelegramIDs    []int64
=======
	AdminTelegramIDs    []string
>>>>>>> Stashed changes

	Warnings []string
}

func (c Config) SecureCookies() bool { return strings.HasPrefix(c.BaseURL, "https://") }

func Load() (Config, error) {
	c := Config{
		Listen:         env("LISTEN_ADDR", ":8080"),
		BaseURL:        strings.TrimRight(env("BASE_URL", "http://localhost:8080"), "/"),
		TimeZone:       env("TZ_NAME", "Asia/Tashkent"),
		DataDir:        env("DATA_DIR", "./data"),
		DatabasePath:   os.Getenv("DATABASE_PATH"),
		TelegramToken:  os.Getenv("TELEGRAM_BOT_TOKEN"),
		RailwayBaseURL: strings.TrimRight(env("RAILWAY_BASE", "https://eticket.railway.uz"), "/"),
		RailwayLang:    env("RAILWAY_LANG", "uz"),
		RailwayCookie:  os.Getenv("RAILWAY_COOKIE"),
		StationsFile:   os.Getenv("STATIONS_FILE"),
	}

	var errs []error
	if c.DatabasePath == "" {
		c.DatabasePath = filepath.Join(c.DataDir, "watcher.db")
	}
	if c.TelegramToken == "" {
<<<<<<< Updated upstream
		errs = append(errs, errors.New("TELEGRAM_BOT_TOKEN kerak — kirish ham, bildirishnomalar ham Telegram bot orqali"))
=======
		errs = append(errs, errors.New("TELEGRAM_BOT_TOKEN kerak: kirish Telegram orqali amalga oshadi"))
>>>>>>> Stashed changes
	}

	var err error
	if os.Getenv("POLL_INTERVAL") != "" {
		c.Warnings = append(c.Warnings, "POLL_INTERVAL endi ishlatilmaydi — oraliqlar admin sahifasidagi jadvaldan olinadi (/admin/polling)")
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
<<<<<<< Updated upstream
	for _, e := range strings.Split(os.Getenv("ADMIN_TELEGRAM_IDS"), ",") {
		if e = strings.TrimSpace(e); e == "" {
			continue
=======
	for _, id := range strings.Split(os.Getenv("ADMIN_TELEGRAM_IDS"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			c.AdminTelegramIDs = append(c.AdminTelegramIDs, id)
>>>>>>> Stashed changes
		}
		id, err := strconv.ParseInt(e, 10, 64)
		if err != nil || id <= 0 {
			errs = append(errs, fmt.Errorf("ADMIN_TELEGRAM_IDS: %q — Telegram ID raqam bo'lishi kerak", e))
			continue
		}
		c.AdminTelegramIDs = append(c.AdminTelegramIDs, id)
	}
	if os.Getenv("ADMIN_EMAILS") != "" {
		c.Warnings = append(c.Warnings, "ADMIN_EMAILS endi ishlatilmaydi — ADMIN_TELEGRAM_IDS ga Telegram ID'larni yozing")
	}
<<<<<<< Updated upstream

=======
	if os.Getenv("ADMIN_EMAILS") != "" {
		c.Warnings = append(c.Warnings, "ADMIN_EMAILS endi ishlatilmaydi — adminlar ADMIN_TELEGRAM_IDS (Telegram ID'lar, vergul bilan) orqali beriladi")
	}
>>>>>>> Stashed changes
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
