package domain

import (
	"errors"
	"testing"
	"time"
)

var trains = []Train{
	{Number: "764Ф", Type: "Скоростной", Brand: "Afrosiyob", Departure: "07:28", Cars: []Car{
		{Type: "Сидячий", Class: "2E", FreeSeats: 12, Price: 270000},
		{Type: "Сидячий", Class: "1В", FreeSeats: 0, Price: 520000},
	}},
	{Number: "056Ф", Type: "Пассажирский", Departure: "21:10", Cars: []Car{
		{Type: "Купе", Class: "2К", FreeSeats: 3, Price: 180000},
	}},
}

func TestMatchTrainsFilters(t *testing.T) {
	cases := []struct {
		name   string
		f      Filter
		trains int
	}{
		{"filtrsiz — faqat joyi borlar", Filter{}, 2},
		{"narx", Filter{MaxPrice: 200000}, 1},
		{"joy soni", Filter{MinSeats: 5}, 1},
		{"turi (registrsiz)", Filter{CarType: "afrosiyob"}, 1},
		{"klass", Filter{CarType: "2к"}, 1},
		{"hech narsa", Filter{CarType: "lyuks"}, 0},
	}
	for _, c := range cases {
		if got := len(MatchTrains(c.f, trains)); got != c.trains {
			t.Errorf("%s: %d ta, kutilgan %d", c.name, got, c.trains)
		}
	}
}

func TestNewWatchValidation(t *testing.T) {
	now := time.Now()
	ok := NewWatchParams{Route: Route{From: "1", To: "2", Date: "2026-12-05"}, ChatID: "42"}
	if _, err := NewWatch(ok, "2026-11-23", now); err != nil {
		t.Fatal(err)
	}
	bad := []NewWatchParams{
		{Route: Route{From: "1", To: "1", Date: "2026-12-05"}, ChatID: "42"},
		{Route: Route{From: "1", To: "2", Date: "2026-11-01"}, ChatID: "42"},
		{Route: Route{From: "1", To: "2", Date: "05.12.2026"}, ChatID: "42"},
		{Route: Route{From: "1", To: "2", Date: "2026-12-05"}, ChatID: " "},
	}
	for i, p := range bad {
		var ve *ValidationError
		if _, err := NewWatch(p, "2026-11-23", now); !errors.As(err, &ve) {
			t.Errorf("#%d: ValidationError kutilgan, olindi %v", i, err)
		}
	}
}

func TestWatchNotifyLifecycle(t *testing.T) {
	w := Watch{Active: true}
	now := time.Now()

	m, notify, sig := w.Evaluate(trains)
	if !notify {
		t.Fatal("birinchi topilmada xabar kerak")
	}
	w.RecordResult(now, len(trains), m, sig, notify, nil)

	if _, notify, _ := w.Evaluate(trains); notify {
		t.Fatal("o'zgarish yo'q — takror xabar kerak emas")
	}

	m, notify, sig = w.Evaluate(nil) // joylar tugadi
	w.RecordResult(now, 0, m, sig, notify, nil)
	if w.LastSig != "" {
		t.Fatal("joy tugaganda imzo tozalanishi kerak")
	}
	if _, notify, _ := w.Evaluate(trains); !notify {
		t.Fatal("joy qayta chiqqanda xabar kerak")
	}

	// Xabar yuborilmasa imzo saqlanmaydi — keyingi siklda qayta urinadi.
	w2 := Watch{Active: true}
	m, notify, sig = w2.Evaluate(trains)
	w2.RecordResult(now, 2, m, sig, notify, errors.New("network"))
	if w2.LastSig != "" {
		t.Fatal("xato bo'lsa imzo saqlanmasligi kerak")
	}
}
