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
	owner := User{ID: 7, TelegramID: 42}
	ok := NewWatchParams{Route: Route{From: "1", To: "2", Date: "2026-12-05"}}
	w, err := NewWatch(owner, ok, "2026-11-23", now)
	if err != nil {
		t.Fatal(err)
	}
	if w.UserID != 7 || !w.Active {
		t.Fatalf("egasi/holati noto'g'ri: %+v", w)
	}
	bad := []struct {
		u User
		p NewWatchParams
	}{
		{owner, NewWatchParams{Route: Route{From: "1", To: "1", Date: "2026-12-05"}}},
		{owner, NewWatchParams{Route: Route{From: "1", To: "2", Date: "2026-11-01"}}},
		{owner, NewWatchParams{Route: Route{From: "1", To: "2", Date: "05.12.2026"}}},
		{owner, NewWatchParams{Route: ok.Route, Filter: Filter{MinSeats: 99}}},
	}
	for i, c := range bad {
		var ve *ValidationError
		if _, err := NewWatch(c.u, c.p, "2026-11-23", now); !errors.As(err, &ve) {
			t.Errorf("#%d: ValidationError kutilgan, olindi %v", i, err)
		}
	}
}

<<<<<<< Updated upstream
func TestIdentityValidate(t *testing.T) {
	now := time.Date(2026, 11, 24, 18, 0, 0, 0, time.UTC)
	ok := Identity{TelegramID: 42, FirstName: "Jem", AuthDate: now.Add(-time.Minute)}
	if err := ok.Validate(now); err != nil {
		t.Fatal(err)
	}
	bad := map[string]Identity{
		"ID yo'q":           {AuthDate: now},
		"eskirgan (replay)": {TelegramID: 42, AuthDate: now.Add(-MaxAuthAge - time.Second)},
		"kelajakdan":        {TelegramID: 42, AuthDate: now.Add(10 * time.Minute)},
		"auth_date yo'q":    {TelegramID: 42},
	}
	for name, id := range bad {
		var ve *ValidationError
		if err := id.Validate(now); !errors.As(err, &ve) {
			t.Errorf("%s: ValidationError kutilgan, olindi %v", name, err)
		}
=======
func TestTelegramProfileValidate(t *testing.T) {
	if err := (TelegramProfile{ID: "42"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (TelegramProfile{}).Validate(); err == nil {
		t.Fatal("ID'siz profil rad etilishi kerak")
	}
}

func TestUserIsAdmin(t *testing.T) {
	u := User{TelegramChatID: "42"}
	if !u.IsAdmin([]string{"7", " 42 "}) || u.IsAdmin([]string{"7"}) || (User{}).IsAdmin([]string{""}) {
		t.Fatal("admin tekshiruvi noto'g'ri")
>>>>>>> Stashed changes
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
