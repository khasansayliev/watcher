// Package usecase — ilova biznes stsenariylari. Tashqi dunyo bilan faqat
// shu fayldagi portlar (interfeyslar) orqali gaplashadi; ularni adapter qatlami amalga oshiradi.
package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/khasansayliev/railbot/internal/domain"
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
}

type UserRepository interface {
	// UpsertByIdentity Google sub bo'yicha foydalanuvchini yaratadi yoki profilini yangilaydi.
	UpsertByIdentity(ctx context.Context, id domain.Identity, now time.Time) (domain.User, error)
	Get(ctx context.Context, id int64) (domain.User, error)
	// SetTelegramChat chatID ni bog'laydi ("" = uzish). Shu chat boshqa userda bo'lsa, undan olinadi.
	SetTelegramChat(ctx context.Context, userID int64, chatID string) error
}

type SessionRepository interface {
	Create(ctx context.Context, tokenHash []byte, userID int64, expiresAt time.Time) error
	// UserByToken amal qilayotgan sessiya egasini qaytaradi; bo'lmasa domain.ErrUnauthorized.
	UserByToken(ctx context.Context, tokenHash []byte, now time.Time) (domain.User, error)
	Delete(ctx context.Context, tokenHash []byte) error
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
}

type LinkCodeRepository interface {
	Create(ctx context.Context, code string, userID int64, expiresAt time.Time) error
	// Consume kodni bir martalik ishlatadi; yaroqsiz/eskirgan bo'lsa domain.ErrNotFound.
	Consume(ctx context.Context, code string, now time.Time) (userID int64, err error)
}

// ---- Tashqi xizmat portlari ----

// TrainSearcher — bo'sh joylar manbai (temir yo'l API).
type TrainSearcher interface {
	Search(ctx context.Context, r domain.Route) ([]domain.Train, error)
}

// IdentityProvider — OAuth provayder (Google).
type IdentityProvider interface {
	AuthURL(state string) string
	Exchange(ctx context.Context, code string) (domain.Identity, error)
}

// Notifier — foydalanuvchiga xabar yetkazish (Telegram).
type Notifier interface {
	Enabled() bool
	NotifyAvailability(ctx context.Context, chatID string, r domain.Route, matches []domain.Match) error
	SendText(ctx context.Context, chatID, text string) error
	// LinkURL — foydalanuvchi bosganda botni kod bilan ochadigan havola (t.me/<bot>?start=<code>).
	LinkURL(code string) (string, error)
}

type StationDirectory interface {
	All() []domain.Station
	Name(code string) string
}

type Clock interface {
	Now() time.Time
	Today() string // domain.DateLayout
}

// UpstreamError — tashqi API xatosi. Monitor RetryAfter'ga qarab pauza qiladi.
type UpstreamError struct {
	Status     int
	RetryAfter time.Duration
	Msg        string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("tashqi API HTTP %d: %s", e.Status, e.Msg)
}
