package domain

import (
	"strings"
	"time"
)

// User — platforma foydalanuvchisi. Telegram orqali kiradi va ro'yxatdan o'tadi:
// alohida parol yoki email yo'q, shaxs — Telegram akkaunti.
type User struct {
	ID int64
	// TelegramChatID — Telegram foydalanuvchi ID'si (shaxsiy chatda chat ID bilan bir xil).
	// Bo'sh bo'lmaydi: xabarlar ham shu chatga yuboriladi.
	TelegramChatID string
	Username       string // @username (bo'lmasligi mumkin)
	Name           string
	CreatedAt      time.Time
}

func (u User) HasTelegram() bool { return u.TelegramChatID != "" }

func (u User) DisplayName() string {
	switch {
	case u.Name != "":
		return u.Name
	case u.Username != "":
		return "@" + u.Username
	}
	return "Foydalanuvchi " + u.TelegramChatID
}

// TelegramProfile — botga yozgan odamning Telegram'dagi ma'lumoti.
type TelegramProfile struct {
	ID       string // Telegram user ID
	Username string
	Name     string
}

func (p TelegramProfile) Validate() error {
	if p.ID == "" {
		return NewValidationError("Telegram akkaunt ma'lumoti to'liq emas")
	}
	return nil
}

// IsAdmin — Telegram ID administratorlar ro'yxatida bormi.
func (u User) IsAdmin(adminIDs []string) bool {
	for _, id := range adminIDs {
		if u.TelegramChatID != "" && strings.TrimSpace(id) == u.TelegramChatID {
			return true
		}
	}
	return false
}
