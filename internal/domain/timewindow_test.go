package domain

import (
	"errors"
	"testing"
	"time"
)

func TestTimeWindowContains(t *testing.T) {
	cases := []struct {
		name string
		w    TimeWindow
		dep  string
		want bool
	}{
		{"cheklovsiz", TimeWindow{}, "03:10", true},
		{"dan: keyin", TimeWindow{From: "08:00"}, "08:37", true},
		{"dan: aynan chegara", TimeWindow{From: "08:00"}, "08:00", true},
		{"dan: oldin", TimeWindow{From: "08:00"}, "07:59", false},
		{"gacha: oldin", TimeWindow{To: "18:00"}, "17:59", true},
		{"gacha: aynan chegara", TimeWindow{To: "18:00"}, "18:00", true},
		{"gacha: keyin", TimeWindow{To: "18:00"}, "18:01", false},
		{"oraliq ichida", TimeWindow{From: "08:00", To: "12:00"}, "10:30", true},
		{"oraliqdan oldin", TimeWindow{From: "08:00", To: "12:00"}, "07:00", false},
		{"oraliqdan keyin", TimeWindow{From: "08:00", To: "12:00"}, "21:10", false},
		{"API formati (sana bilan)", TimeWindow{From: "08:00"}, "01.12.2026 08:37", true},
		{"API formati: kechqurun", TimeWindow{To: "12:00"}, "01.12.2026 21:10", false},
		{"bir xonali soat", TimeWindow{From: "08:00"}, "9:05", true},
		{"o'qib bo'lmaydi — yashirilmaydi", TimeWindow{From: "08:00"}, "noma'lum", true},
	}
	for _, c := range cases {
		if got := c.w.Contains(c.dep); got != c.want {
			t.Errorf("%s: Contains(%q)=%v, kutilgan %v", c.name, c.dep, got, c.want)
		}
	}
}

func TestTimeWindowValidate(t *testing.T) {
	ok := []TimeWindow{{}, {From: "00:00"}, {To: "23:59"}, {From: "08:00", To: "08:00"}, {From: "08:00", To: "18:30"}}
	for _, w := range ok {
		if err := w.Validate(); err != nil {
			t.Errorf("%+v: kutilmagan xato %v", w, err)
		}
	}
	bad := []TimeWindow{{From: "8:00"}, {To: "24:00"}, {From: "12:60"}, {From: "abc"}, {From: "18:00", To: "08:00"}}
	for _, w := range bad {
		var ve *ValidationError
		if err := w.Validate(); !errors.As(err, &ve) {
			t.Errorf("%+v: ValidationError kutilgan, olindi %v", w, err)
		}
	}
}

func TestMatchTrainsRespectsWindow(t *testing.T) {
	// trains: 764Ф 07:28 (joy bor), 056Ф 21:10 (joy bor)
	if got := MatchTrains(Filter{Window: TimeWindow{From: "12:00"}}, trains); len(got) != 1 || got[0].Train.Number != "056Ф" {
		t.Fatalf("12:00 dan keyin: %+v", got)
	}
	if got := MatchTrains(Filter{Window: TimeWindow{To: "12:00"}}, trains); len(got) != 1 || got[0].Train.Number != "764Ф" {
		t.Fatalf("12:00 gacha: %+v", got)
	}
	if got := MatchTrains(Filter{Window: TimeWindow{From: "07:00", To: "22:00"}}, trains); len(got) != 2 {
		t.Fatalf("keng oraliq: %+v", got)
	}
	if got := MatchTrains(Filter{Window: TimeWindow{From: "10:00", To: "20:00"}}, trains); len(got) != 0 {
		t.Fatalf("hech kim tushmaydi: %+v", got)
	}
	if got := (TimeWindow{From: "12:00"}).FilterTrains(trains); len(got) != 1 {
		t.Fatalf("FilterTrains: %+v", got)
	}
}

func TestNewWatchRejectsBadWindow(t *testing.T) {
	owner := User{ID: 1, TelegramChatID: "42"}
	p := NewWatchParams{Route: Route{From: "1", To: "2", Date: "2026-12-05"}, Filter: Filter{Window: TimeWindow{From: "18:00", To: "08:00"}}}
	var ve *ValidationError
	if _, err := NewWatch(owner, p, "2026-11-23", time.Now()); !errors.As(err, &ve) {
		t.Fatalf("teskari oraliq rad etilishi kerak: %v", err)
	}
	p.Filter.Window = TimeWindow{From: " 08:00 "}
	w, err := NewWatch(owner, p, "2026-11-23", time.Now())
	if err != nil || w.Filter.Window.From != "08:00" {
		t.Fatalf("bo'shliqlar kesilishi kerak: %+v %v", w.Filter.Window, err)
	}
}
