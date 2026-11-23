// Package textfmt — qatlamlardan mustaqil kichik formatlash yordamchilari.
package textfmt

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Truncate satrni n ta belgigacha qisqartiradi.
func Truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// Sum: 270000 -> "270 000"
func Sum(v float64) string {
	s := strconv.FormatInt(int64(v+0.5), 10)
	var b strings.Builder
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(ch)
	}
	return b.String()
}

// Ago — "5 daq oldin" ko'rinishidagi nisbiy vaqt.
func Ago(t, now time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "hozirgina"
	case d < time.Hour:
		return fmt.Sprintf("%d daq oldin", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d soat oldin", int(d.Hours()))
	default:
		return t.Format("02.01 15:04")
	}
}

// Interval: 10s -> "10 soniya", 5m -> "5 daqiqa", 1h30m -> "1 soat 30 daqiqa", 48h -> "2 kun".
func Interval(d time.Duration) string {
	switch {
	case d <= 0:
		return "—"
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%d kun", d/(24*time.Hour))
	case d%time.Hour == 0:
		return fmt.Sprintf("%d soat", d/time.Hour)
	case d >= time.Hour && d%time.Minute == 0:
		return fmt.Sprintf("%d soat %d daqiqa", d/time.Hour, (d%time.Hour)/time.Minute)
	case d%time.Minute == 0:
		return fmt.Sprintf("%d daqiqa", d/time.Minute)
	default:
		return fmt.Sprintf("%d soniya", d/time.Second)
	}
}
