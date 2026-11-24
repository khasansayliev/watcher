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

func TestSettingsCheckCreate(t *testing.T) {
	plain, allowed := User{}, User{CanCreate: true}
	cases := []struct {
		mode  CreationMode
		u     User
		admin bool
		ok    bool
	}{
		{CreationApproved, plain, false, false},
		{CreationApproved, allowed, false, true},
		{CreationApproved, plain, true, true},
		{CreationAll, plain, false, true},
		{CreationNone, allowed, false, false},
		{CreationNone, plain, true, true},
		{"", plain, false, false}, // noma'lum qiymat xavfsiz tomonga: ruxsatsiz
	}
	for i, c := range cases {
		err := Settings{CreationMode: c.mode}.CheckCreate(c.u, c.admin)
		if (err == nil) != c.ok {
			t.Errorf("#%d %s admin=%v can=%v: err=%v, kutilgan ok=%v", i, c.mode, c.admin, c.u.CanCreate, err, c.ok)
		}
	}
	if !DefaultSettings().CreationMode.Valid() || DefaultSettings().CreationMode != CreationApproved {
		t.Fatal("standart rejim 'approved' bo'lishi kerak")
	}
}

func TestWatchIntervalAndValidation(t *testing.T) {
	p := DefaultPollingPolicy()
	w := Watch{Route: Route{Date: "2026-12-05"}}
	if got := w.IntervalIn(p, "2026-11-23"); got != time.Hour { // 12 kun: "30 kundan kam"
		t.Fatalf("jadval: %s", got)
	}
	w.CheckEvery = 45 * time.Second
	if got := w.IntervalIn(p, "2026-11-23"); got != 45*time.Second {
		t.Fatalf("alohida: %s", got)
	}
	for _, d := range []time.Duration{0, MinCheckInterval, MaxCheckInterval} {
		if err := ValidateCheckEvery(d); err != nil {
			t.Errorf("%s: %v", d, err)
		}
	}
	for _, d := range []time.Duration{time.Second, MinCheckInterval - 1, MaxCheckInterval + 1, -time.Hour} {
		var ve *ValidationError
		if err := ValidateCheckEvery(d); !errors.As(err, &ve) {
			t.Errorf("%s rad etilishi kerak: %v", d, err)
		}
	}
}

func TestApplyEdit(t *testing.T) {
	w := Watch{
		Route: Route{From: "1", To: "2", Date: "2026-12-05"}, Active: true, AdminDisabled: true, CheckEvery: time.Minute,
		LastCheck: time.Now(), LastSig: "x", LastMatches: 3, LastNotified: time.Now(),
	}
	in := NewWatchParams{Route: Route{From: "1", To: "3", Date: "2026-12-08"}, Filter: Filter{CarType: " Купе "}}
	if err := w.ApplyEdit(in, "2026-11-24"); err != nil {
		t.Fatal(err)
	}
	if w.Route.To != "3" || w.Filter.CarType != "Купе" || !w.LastCheck.IsZero() || w.LastSig != "" || w.LastMatches != 0 {
		t.Fatalf("%+v", w)
	}
	if !w.AdminDisabled || w.CheckEvery != time.Minute || w.LastNotified.IsZero() {
		t.Fatalf("admin belgilari va oxirgi xabar vaqti saqlanishi kerak: %+v", w)
	}
	before := w
	var ve *ValidationError
	if err := w.ApplyEdit(NewWatchParams{Route: Route{From: "1", To: "1", Date: "2026-12-08"}}, "2026-11-24"); !errors.As(err, &ve) {
		t.Fatal("noto'g'ri tahrir rad etilishi kerak")
	}
	if w != before {
		t.Fatal("rad etilgan tahrir kuzatuvni o'zgartirmasligi kerak")
	}
}
