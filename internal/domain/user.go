package domain

import (
	"strings"
	"time"
)

// User — platforma foydalanuvchisi (Google akkaunti orqali kiradi).
type User struct {
	ID             int64
	GoogleSub      string
	Email          string
	Name           string
	AvatarURL      string
	TelegramChatID string // bo'sh = Telegram ulanmagan
	CreatedAt      time.Time
}

func (u User) HasTelegram() bool { return u.TelegramChatID != "" }

func (u User) DisplayName() string {
	if u.Name != "" {
		return u.Name
	}
	return u.Email
}

// Identity — tashqi provayder (Google) tasdiqlagan shaxs ma'lumoti.
type Identity struct {
	Subject       string // Google'dagi o'zgarmas ID (sub)
	Email         string
	EmailVerified bool
	Name          string
	Picture       string
}

func (i Identity) Validate() error {
	if i.Subject == "" || i.Email == "" {
		return NewValidationError("Google akkaunt ma'lumoti to'liq emas")
	}
	if !i.EmailVerified {
		return NewValidationError("Google akkauntingizda email tasdiqlanmagan")
	}
	return nil
}

// IsAdmin — email administratorlar ro'yxatida bormi.
func (u User) IsAdmin(adminEmails []string) bool {
	for _, e := range adminEmails {
		if strings.EqualFold(strings.TrimSpace(e), u.Email) {
			return true
		}
	}
	return false
}
