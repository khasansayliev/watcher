package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/khasansayliev/railbot/internal/pkg/textfmt"
)

// Watch — foydalanuvchining kuzatuvi (asosiy entity).
type Watch struct {
	ID        string
	Route     Route
	Filter    Filter
	ChatID    string
	Active    bool
	CreatedAt time.Time

	LastCheck    time.Time
	LastStatus   string
	LastMatches  int
	LastSig      string
	LastNotified time.Time
}

type NewWatchParams struct {
	Route  Route
	Filter Filter
	ChatID string
}

// NewWatch biznes qoidalarini tekshirib yangi faol kuzatuv yaratadi.
func NewWatch(p NewWatchParams, today string, now time.Time) (Watch, error) {
	p.ChatID = strings.TrimSpace(p.ChatID)
	p.Filter.CarType = strings.TrimSpace(p.Filter.CarType)
	if err := p.Route.Validate(today); err != nil {
		return Watch{}, err
	}
	if p.ChatID == "" {
		return Watch{}, NewValidationError("Telegram chat ID kiriting")
	}
	if p.Filter.MaxPrice < 0 {
		return Watch{}, NewValidationError("Maks. narx manfiy bo'lmasin")
	}
	if p.Filter.MinSeats < 0 || p.Filter.MinSeats > 50 {
		return Watch{}, NewValidationError("Joylar soni 1–50 oralig'ida bo'lsin")
	}
	return Watch{Route: p.Route, Filter: p.Filter, ChatID: p.ChatID, Active: true, CreatedAt: now}, nil
}

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
