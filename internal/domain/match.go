package domain

import (
	"sort"
	"strings"
)

// Filter — kuzatuv shartlari. Nol qiymat = cheklov yo'q.
type Filter struct {
	MaxPrice float64
	MinSeats int
	CarType  string     // poyezd/vagon turi yoki klass bo'yicha qism-satr
	Window   TimeWindow // kun ichida jo'nash vaqti oralig'i
}

// Match — shartga mos vagonlari bor poyezd.
type Match struct {
	Train Train
	Cars  []Car
}

// MatchTrains filtrga mos poyezd va vagonlarni qaytaradi.
func MatchTrains(f Filter, trains []Train) []Match {
	minSeats := f.MinSeats
	if minSeats < 1 {
		minSeats = 1
	}
	needle := strings.ToLower(strings.TrimSpace(f.CarType))

	var out []Match
	for _, t := range trains {
		if !f.Window.Contains(t.Departure) {
			continue
		}
		var cars []Car
		for _, c := range t.Cars {
			if c.FreeSeats < minSeats {
				continue
			}
			if f.MaxPrice > 0 && c.Price > 0 && c.Price > f.MaxPrice {
				continue
			}
			if needle != "" && !strings.Contains(strings.ToLower(c.Type+" "+c.Class+" "+t.Type+" "+t.Brand), needle) {
				continue
			}
			cars = append(cars, c)
		}
		if len(cars) > 0 {
			out = append(out, Match{Train: t, Cars: cars})
		}
	}
	return out
}

// Signature — mos joylar to'plamining izi. Joylar soni kirmaydi:
// xabar faqat yangi poyezd/vagon turi paydo bo'lganda yuboriladi.
func Signature(ms []Match) string {
	set := map[string]struct{}{}
	for _, m := range ms {
		for _, c := range m.Cars {
			set[m.Train.Number+"|"+m.Train.Departure+"|"+c.Type+"|"+c.Class] = struct{}{}
		}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ";")
}
