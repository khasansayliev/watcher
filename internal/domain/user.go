package domain

import (
	"strconv"
	"strings"
	"time"
)

<<<<<<< Updated upstream
// User — platforma foydalanuvchisi. Telegram orqali kiradi; bildirishnomalar ham
// o'sha Telegram akkauntiga (bot bilan shaxsiy chat) yuboriladi.
type User struct {
	ID         int64
	TelegramID int64 // Telegram foydalanuvchi ID'si; botning shaxsiy chat ID'si ham shu
	Username   string
	Name       string
	PhotoURL   string
	CreatedAt  time.Time
=======
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
>>>>>>> Stashed changes
}

// ChatID — bildirishnoma yuboriladigan chat.
func (u User) ChatID() string {
	if u.TelegramID == 0 {
		return ""
	}
	return strconv.FormatInt(u.TelegramID, 10)
}

func (u User) Handle() string {
	if u.Username != "" {
		return "@" + u.Username
	}
	return ""
}

func (u User) DisplayName() string {
	switch {
	case u.Name != "":
		return u.Name
	case u.Username != "":
		return "@" + u.Username
<<<<<<< Updated upstream
	default:
		return "ID " + strconv.FormatInt(u.TelegramID, 10)
	}
}

// IsAdmin — Telegram ID administratorlar ro'yxatida bormi.
// (Username o'zgarishi mumkin, ID esa o'zgarmaydi.)
func (u User) IsAdmin(adminIDs []int64) bool {
	for _, id := range adminIDs {
		if id != 0 && id == u.TelegramID {
=======
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
>>>>>>> Stashed changes
			return true
		}
	}
	return false
}

// MaxAuthAge — Telegram imzolagan kirish ma'lumotining eng uzoq yaroqlilik muddati.
// Eskirgan ma'lumot qayta ishlatilishining (replay) oldini oladi.
const MaxAuthAge = time.Hour

// Identity — Telegram tasdiqlagan (imzosi tekshirilgan) foydalanuvchi ma'lumoti.
type Identity struct {
	TelegramID int64
	FirstName  string
	LastName   string
	Username   string
	PhotoURL   string
	AuthDate   time.Time
}

func (i Identity) FullName() string {
	return strings.TrimSpace(i.FirstName + " " + i.LastName)
}

// Validate imzodan tashqari biznes qoidalarini tekshiradi: ID va kirish vaqti.
func (i Identity) Validate(now time.Time) error {
	if i.TelegramID <= 0 {
		return NewValidationError("Telegram ma'lumoti to'liq emas")
	}
	if i.AuthDate.IsZero() || now.Sub(i.AuthDate) > MaxAuthAge {
		return NewValidationError("Kirish havolasi eskirgan — qaytadan \"Telegram orqali kirish\" ni bosing")
	}
	if i.AuthDate.Sub(now) > 5*time.Minute {
		return NewValidationError("Kirish vaqti noto'g'ri — qurilmangiz soatini tekshiring")
	}
	return nil
}
