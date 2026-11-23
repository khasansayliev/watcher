// Package usecase — ilova biznes stsenariylari. Tashqi dunyo bilan faqat
// shu fayldagi portlar (interfeyslar) orqali gaplashadi; ularni adapter qatlami amalga oshiradi.
package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/khasansayliev/railbot/internal/domain"
)

// WatchRepository — kuzatuvlarni saqlash.
type WatchRepository interface {
	Create(ctx context.Context, w domain.Watch) (domain.Watch, error)
	Get(ctx context.Context, id string) (domain.Watch, error)
	List(ctx context.Context) ([]domain.Watch, error)
	// Update yozuvni atomar o'zgartiradi (o'qish-o'zgartirish-yozish bitta qulf ostida).
	Update(ctx context.Context, id string, fn func(*domain.Watch)) (domain.Watch, error)
	Delete(ctx context.Context, id string) error
}

// TrainSearcher — bo'sh joylar manbai (temir yo'l API).
type TrainSearcher interface {
	Search(ctx context.Context, r domain.Route) ([]domain.Train, error)
}

// Notifier — foydalanuvchiga xabar yetkazish (Telegram).
type Notifier interface {
	Enabled() bool
	NotifyAvailability(ctx context.Context, chatID string, r domain.Route, matches []domain.Match) error
	SendTest(ctx context.Context, chatID string) error
}

// StationDirectory — stansiyalar ma'lumotnomasi.
type StationDirectory interface {
	All() []domain.Station
	Name(code string) string
}

// Clock — vaqt manbai (testlarda almashtiriladi).
type Clock interface {
	Now() time.Time
	Today() string // domain.DateLayout
}

// UpstreamError — tashqi API xatosi. Adapterlar shu turda qaytaradi,
// Monitor esa RetryAfter'ga qarab pauza qiladi.
type UpstreamError struct {
	Status     int
	RetryAfter time.Duration
	Msg        string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("tashqi API HTTP %d: %s", e.Status, e.Msg)
}
