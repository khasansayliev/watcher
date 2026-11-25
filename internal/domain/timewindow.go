package domain

import (
	"regexp"
	"strings"
)

// TimeWindow — kun ichida jo'nash vaqti oralig'i ("HH:MM", ikkala chegara ham kiradi).
// Bo'sh chegara = cheklov yo'q:
//
//	From="08:00"             — 08:00 dan keyingilar
//	To="18:00"               — 18:00 gacha bo'lganlar
//	From="08:00", To="18:00" — shu oraliqdagilar
//	ikkalasi bo'sh           — kun bo'yicha hammasi
type TimeWindow struct {
	From string
	To   string
}

var (
	hhmm      = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	clockInTx = regexp.MustCompile(`(\d{1,2}):(\d{2})`)
)

func (w TimeWindow) IsZero() bool { return w.From == "" && w.To == "" }

// Validate chegaralar formatini ("HH:MM") va tartibini tekshiradi.
func (w TimeWindow) Validate() error {
	for _, v := range []string{w.From, w.To} {
		if v != "" && !hhmm.MatchString(v) {
			return NewValidationError("Vaqt SS:DD formatida bo'lsin, masalan 08:30")
		}
	}
	if w.From != "" && w.To != "" && w.From > w.To {
		return NewValidationError("Boshlanish vaqti tugash vaqtidan keyin bo'lmasin")
	}
	return nil
}

// Contains jo'nash vaqti oraliqqa tushishini aytadi. departure: "07:28" yoki "01.12.2026 08:37"
// kabi satr (oxirgi SS:DD olinadi). Vaqtni o'qib bo'lmasa, poyezd yashirilmaydi (xabar o'tkazib yuborilmasin).
func (w TimeWindow) Contains(departure string) bool {
	if w.IsZero() {
		return true
	}
	all := clockInTx.FindAllStringSubmatch(departure, -1)
	if len(all) == 0 {
		return true
	}
	m := all[len(all)-1]
	if len(m[1]) == 1 {
		m[1] = "0" + m[1]
	}
	t := m[1] + ":" + m[2]
	if w.From != "" && t < w.From {
		return false
	}
	if w.To != "" && t > w.To {
		return false
	}
	return true
}

// Label foydalanuvchiga ko'rsatiladigan qisqa matn ("" = cheklov yo'q).
func (w TimeWindow) Label() string {
	switch {
	case w.From != "" && w.To != "":
		return w.From + "–" + w.To
	case w.From != "":
		return w.From + " dan keyin"
	case w.To != "":
		return w.To + " gacha"
	}
	return ""
}

// FilterTrains oraliqqa tushmaydigan poyezdlarni tashlab yuboradi.
func (w TimeWindow) FilterTrains(ts []Train) []Train {
	if w.IsZero() {
		return ts
	}
	out := make([]Train, 0, len(ts))
	for _, t := range ts {
		if w.Contains(t.Departure) {
			out = append(out, t)
		}
	}
	return out
}

func trimWindow(w TimeWindow) TimeWindow {
	return TimeWindow{From: strings.TrimSpace(w.From), To: strings.TrimSpace(w.To)}
}
