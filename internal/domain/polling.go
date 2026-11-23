package domain

import (
	"fmt"
	"sort"
	"time"

	"github.com/khasansayliev/watcher/internal/pkg/textfmt"
)

const (
	// MinCheckInterval — admin qo'ya oladigan eng qisqa oraliq.
	MinCheckInterval = 10 * time.Second
	// MaxCheckInterval — eng uzun oraliq (bundan uzunida kuzatuvning ma'nosi qolmaydi).
	MaxCheckInterval = 7 * 24 * time.Hour
)

// PollingRule — "jo'nashga MaxDaysLeft kundan kam qolgan bo'lsa, har Interval da tekshir".
// MaxDaysLeft == 0 — "qolganlari" (hech bir chegaraga tushmaganlar).
type PollingRule struct {
	MaxDaysLeft int
	Interval    time.Duration
}

func (r PollingRule) IsDefault() bool { return r.MaxDaysLeft == 0 }

func (r PollingRule) Label() string {
	if r.IsDefault() {
		return "Qolganlari"
	}
	return fmt.Sprintf("%d kundan kam", r.MaxDaysLeft)
}

// PollingPolicy — tekshiruv oraliqlari jadvali. Qoidalar chegarasi bo'yicha o'sish tartibida,
// "qolganlari" qatori oxirida turadi.
type PollingPolicy struct {
	Rules     []PollingRule
	UpdatedAt time.Time
	UpdatedBy string
}

// DefaultPollingPolicy — boshlang'ich jadval (migratsiya ham shu qiymatlarni yozadi).
func DefaultPollingPolicy() PollingPolicy {
	p, _ := NewPollingPolicy([]PollingRule{
		{MaxDaysLeft: 2, Interval: 10 * time.Second},
		{MaxDaysLeft: 10, Interval: 5 * time.Minute},
		{MaxDaysLeft: 30, Interval: time.Hour},
		{MaxDaysLeft: 0, Interval: 12 * time.Hour},
	})
	return p
}

// NewPollingPolicy qoidalarni tekshiradi va tartiblaydi.
func NewPollingPolicy(rules []PollingRule) (PollingPolicy, error) {
	seen := map[int]bool{}
	defaults := 0
	for _, r := range rules {
		switch {
		case r.MaxDaysLeft < 0 || r.MaxDaysLeft > 365:
			return PollingPolicy{}, NewValidationError("Kunlar soni 1–365 oralig'ida bo'lsin")
		case seen[r.MaxDaysLeft] && !r.IsDefault():
			return PollingPolicy{}, NewValidationError(fmt.Sprintf("%d kunlik chegara ikki marta yozilgan", r.MaxDaysLeft))
		case r.Interval < MinCheckInterval:
			return PollingPolicy{}, NewValidationError(fmt.Sprintf("%s: oraliq kamida %s bo'lishi kerak", r.Label(), textfmt.Interval(MinCheckInterval)))
		case r.Interval > MaxCheckInterval:
			return PollingPolicy{}, NewValidationError(fmt.Sprintf("%s: oraliq ko'pi bilan 7 kun bo'lishi mumkin", r.Label()))
		}
		seen[r.MaxDaysLeft] = true
		if r.IsDefault() {
			defaults++
		}
	}
	if defaults != 1 {
		return PollingPolicy{}, NewValidationError("\"Qolganlari\" qatori aynan bitta bo'lishi kerak")
	}
	sorted := append([]PollingRule(nil), rules...)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.IsDefault() != b.IsDefault() {
			return b.IsDefault()
		}
		return a.MaxDaysLeft < b.MaxDaysLeft
	})
	return PollingPolicy{Rules: sorted}, nil
}

// RuleFor qolgan kunlar soniga mos qoidani qaytaradi.
func (p PollingPolicy) RuleFor(daysLeft int) PollingRule {
	for _, r := range p.Rules {
		if r.IsDefault() || daysLeft < r.MaxDaysLeft {
			return r
		}
	}
	return PollingRule{Interval: 12 * time.Hour} // bo'sh siyosat — ehtiyot chorasi
}

// IntervalFor sana bo'yicha tekshiruv oralig'i.
func (p PollingPolicy) IntervalFor(travelDate, today string) time.Duration {
	return p.RuleFor(DaysLeft(travelDate, today)).Interval
}

// DaysLeft — bugundan sanagacha butun kunlar (bugun = 0, ertaga = 1).
func DaysLeft(travelDate, today string) int {
	d, err1 := time.Parse(DateLayout, travelDate)
	t, err2 := time.Parse(DateLayout, today)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(d.Sub(t).Hours() / 24)
}
