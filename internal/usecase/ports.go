// Package usecase — ilova biznes stsenariylari. Tashqi dunyo bilan faqat
// shu fayldagi portlar (interfeyslar) orqali gaplashadi; ularni adapter qatlami amalga oshiradi.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
)

// ---- Saqlash portlari ----

type WatchRepository interface {
	Create(ctx context.Context, w domain.Watch) (domain.Watch, error)
	Get(ctx context.Context, id int64) (domain.Watch, error)
	ListByUser(ctx context.Context, userID int64) ([]domain.Watch, error)
	ListActive(ctx context.Context) ([]domain.Watch, error) // barcha foydalanuvchilar
	CountActiveByUser(ctx context.Context, userID int64) (int, error)
	// Update yozuvni atomar o'zgartiradi (o'qish-o'zgartirish-yozish bitta tranzaksiyada).
	Update(ctx context.Context, id int64, fn func(*domain.Watch)) (domain.Watch, error)
	Delete(ctx context.Context, id int64) error
	// ListAll admin uchun: barcha kuzatuvlar (userID != 0 — faqat shu foydalanuvchiniki), eng yangisi birinchi.
	ListAll(ctx context.Context, userID int64, limit, offset int) ([]domain.WatchWithOwner, int, error)
}

type UserRepository interface {
	// UpsertByTelegram Telegram ID bo'yicha foydalanuvchini yaratadi (ro'yxatdan o'tish)
	// yoki profilini yangilaydi (kirish).
	UpsertByTelegram(ctx context.Context, p domain.TelegramProfile, now time.Time) (domain.User, error)
	Get(ctx context.Context, id int64) (domain.User, error)
	// List admin uchun: barcha foydalanuvchilar statistikasi bilan, eng yangisi birinchi.
	List(ctx context.Context, limit, offset int) ([]domain.UserSummary, int, error)
	SetCanCreate(ctx context.Context, id int64, allow bool) error
	SetDisabled(ctx context.Context, id int64, disabled bool) error
}

// SettingsRepository — platforma sozlamalari (admin boshqaradi).
type SettingsRepository interface {
	Get(ctx context.Context) (domain.Settings, error)
	SetCreationMode(ctx context.Context, m domain.CreationMode) error
}

type SessionRepository interface {
	Create(ctx context.Context, tokenHash []byte, userID int64, expiresAt time.Time) error
	// UserByToken amal qilayotgan sessiya egasini qaytaradi; bo'lmasa domain.ErrUnauthorized.
	UserByToken(ctx context.Context, tokenHash []byte, now time.Time) (domain.User, error)
	Delete(ctx context.Context, tokenHash []byte) error
	// DeleteByUser foydalanuvchining barcha sessiyalarini o'chiradi (akkaunt o'chirilganda).
	DeleteByUser(ctx context.Context, userID int64) error
	// DeleteByTelegram foydalanuvchining barcha sessiyalarini o'chiradi.
	DeleteByTelegram(ctx context.Context, telegramID string) error
	// DeleteExpired eskirgan sessiyalar va kirish so'rovlarini o'chiradi.
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
}

// LoginRequestRepository — bir martalik kirish so'rovlari. Brauzer so'rov ochadi (Begin),
// foydalanuvchi botda tasdiqlaydi (Approve), brauzer natijani oladi (Claim).
type LoginRequestRepository interface {
	Begin(ctx context.Context, code string, expiresAt time.Time) error
	// Approve so'rovni foydalanuvchiga bog'laydi; so'rov yo'q/eskirgan/allaqachon tasdiqlangan bo'lsa domain.ErrNotFound.
	Approve(ctx context.Context, code string, userID int64, now time.Time) error
	// Claim tasdiqlangan so'rovni bir martalik oladi va o'chiradi.
	// Hali tasdiqlanmagan bo'lsa ErrLoginPending, yo'q/eskirgan bo'lsa domain.ErrNotFound.
	Claim(ctx context.Context, code string, now time.Time) (userID int64, err error)
}

// LoginCodeRepository — "Telegram'siz qurilma" uchun bir martalik kodlar: bot kodni Telegram tasdiqlagan
// foydalanuvchiga beradi, foydalanuvchi uni saytga kiritadi. Kodning o'zi emas, sha256 xeshi saqlanadi.
type LoginCodeRepository interface {
	// Put kodni saqlaydi; shu Telegram akkauntning oldingi kodlari o'chiriladi (bir vaqtda bitta amal qiluvchi kod).
	Put(ctx context.Context, hash []byte, p domain.TelegramProfile, expiresAt time.Time) error
	// Consume kodni atomar o'chiradi va egasini qaytaradi. Yo'q yoki muddati o'tgan bo'lsa domain.ErrNotFound.
	Consume(ctx context.Context, hash []byte, now time.Time) (domain.TelegramProfile, error)
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
}

// CheckLogRepository — har bir tekshiruv so'rovining yozuvi (kuzatuv loglari).
type CheckLogRepository interface {
	Add(ctx context.Context, l domain.CheckLog) error
	// ListByWatch req_time bo'yicha kamayish tartibida; jami yozuvlar sonini ham qaytaradi.
	ListByWatch(ctx context.Context, watchID int64, limit, offset int) ([]domain.CheckLog, int, error)
	// DeleteBefore t dan eski yozuvlarni o'chiradi.
	DeleteBefore(ctx context.Context, t time.Time) (int64, error)
}

// PollingRuleRepository — tekshiruv oraliqlari jadvali (admin boshqaradi).
type PollingRuleRepository interface {
	Get(ctx context.Context) (domain.PollingPolicy, error)
	Save(ctx context.Context, p domain.PollingPolicy) error
}

// ---- Tashqi xizmat portlari ----

// TrainSearcher — bo'sh joylar manbai (temir yo'l API).
type TrainSearcher interface {
	Search(ctx context.Context, r domain.Route) ([]domain.Train, error)
}

// Notifier — foydalanuvchiga xabar yetkazish (Telegram).
type Notifier interface {
	Enabled() bool
	NotifyAvailability(ctx context.Context, chatID string, r domain.Route, matches []domain.Match) error
	SendText(ctx context.Context, chatID, text string) error
	// BotURL — bosilganda botni kod bilan ochadigan havola (t.me/<bot>?start=<code>).
	BotURL(code string) (string, error)
}

type StationDirectory interface {
	All() []domain.Station
	Name(code string) string
}

type Clock interface {
	Now() time.Time
	Today() string // domain.DateLayout
}

// ErrLoginPending — kirish so'rovi hali botda tasdiqlanmagan.
var ErrLoginPending = errors.New("kirish tasdiqlanmagan")

// UpstreamError — tashqi API xatosi. Monitor RetryAfter'ga qarab pauza qiladi.
type UpstreamError struct {
	Status     int
	RetryAfter time.Duration
	Msg        string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("tashqi API HTTP %d: %s", e.Status, e.Msg)
}
