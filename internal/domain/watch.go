package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/khasansayliev/watcher/internal/pkg/textfmt"
)

// Watch — foydalanuvchining kuzatuvi (asosiy entity).
type Watch struct {
	ID     int64
	UserID int64
	Route  Route
	Filter Filter
	Active bool
	// AdminDisabled — admin to'xtatgan; foydalanuvchi uni o'zi qayta yoqa olmaydi.
	AdminDisabled bool
	CreatedAt     time.Time

	LastCheck    time.Time
	LastStatus   string
	LastMatches  int
	LastSig      string
	LastNotified time.Time
}

type NewWatchParams struct {
	Route  Route
	Filter Filter
}

// NewWatch biznes qoidalarini tekshirib, foydalanuvchiga tegishli yangi faol kuzatuv yaratadi.
func NewWatch(owner User, p NewWatchParams, today string, now time.Time) (Watch, error) {
	p.Filter.CarType = strings.TrimSpace(p.Filter.CarType)
	p.Filter.Window = trimWindow(p.Filter.Window)
	if !owner.HasTelegram() {
		return Watch{}, NewValidationError("Telegram akkauntingiz topilmadi — qaytadan kiring")
	}
	if err := p.Route.Validate(today); err != nil {
		return Watch{}, err
	}
	if p.Filter.MaxPrice < 0 {
		return Watch{}, NewValidationError("Maks. narx manfiy bo'lmasin")
	}
	if p.Filter.MinSeats < 0 || p.Filter.MinSeats > 50 {
		return Watch{}, NewValidationError("Joylar soni 1–50 oralig'ida bo'lsin")
	}
	if err := p.Filter.Window.Validate(); err != nil {
		return Watch{}, err
	}
	if len(p.Filter.CarType) > 64 {
		return Watch{}, NewValidationError("Filtr juda uzun")
	}
	return Watch{UserID: owner.ID, Route: p.Route, Filter: p.Filter, Active: true, CreatedAt: now}, nil
}

// Runnable — kuzatuv tekshirilishi kerakmi (foydalanuvchi ham, admin ham to'xtatmagan).
func (w Watch) Runnable() bool { return w.Active && !w.AdminDisabled }

func (w Watch) OwnedBy(userID int64) bool { return w.UserID == userID }

func (w Watch) IsExpired(today string) bool { return w.Route.Date < today }

func (w *Watch) Expire() {
	w.Active = false
	w.LastStatus = "Sana o'tib ketdi — to'xtatildi"
}

// Toggle faollikni almashtiradi. Qayta yoqilganda keyingi topilma albatta xabar qilinadi.
func (w *Watch) Toggle() {
	w.Active = !w.Active
	if w.Active {
		w.LastSig = ""
	}
}

// Evaluate poyezdlar ro'yxatini filtr bo'yicha baholaydi va xabar yuborish kerakmi — aytadi.
func (w Watch) Evaluate(trains []Train) (matches []Match, notify bool, sig string) {
	matches = MatchTrains(w.Filter, trains)
	sig = Signature(matches)
	return matches, len(matches) > 0 && sig != w.LastSig, sig
}

// RecordResult tekshiruv natijasini holatga yozadi.
func (w *Watch) RecordResult(at time.Time, trainCount int, matches []Match, sig string, notified bool, notifyErr error) {
	w.LastCheck = at
	w.LastMatches = len(matches)
	w.LastStatus = fmt.Sprintf("%d ta poyezd, mos: %d", trainCount, len(matches))
	switch {
	case notified && notifyErr == nil:
		w.LastSig = sig
		w.LastNotified = at
	case notified:
		w.LastStatus += " | Telegram xato: " + textfmt.Truncate(notifyErr.Error(), 120)
	case len(matches) == 0:
		w.LastSig = "" // joylar tugadi — qayta chiqsa yana xabar beriladi
	}
}

func (w *Watch) RecordFailure(at time.Time, err error) {
	w.LastCheck = at
	w.LastStatus = "Xato: " + textfmt.Truncate(err.Error(), 160)
}
