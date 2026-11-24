package webui

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

func render(t *testing.T, name string, vm any) string {
	t.Helper()
	s, err := New(Deps{})
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&b, name, vm); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return b.String()
}

var stations = []domain.Station{{Code: "2900000", Name: "Toshkent"}, {Code: "2900700", Name: "Samarqand"}, {Code: "2900800", Name: "Buxoro"}}

func TestDashboardDefaultsToTashkentBukhara(t *testing.T) {
	html := render(t, "index.html", dashboardVM{
		layoutVM: layoutVM{User: &userVM{Name: "Ali"}}, Stations: stations, Today: "2026-12-01",
		DefaultFrom: defaultFrom, DefaultTo: defaultTo,
	})
	if !strings.Contains(html, `<option value="2900000" selected>`) || !strings.Contains(html, `<option value="2900800" selected>`) {
		t.Fatal("standart yo'nalish Toshkent → Buxoro bo'lishi kerak")
	}
	if strings.Count(html, " selected>") != 2 {
		t.Fatal("faqat ikkita standart tanlov bo'lishi kerak")
	}
	for _, f := range []string{`name="time_from"`, `name="time_to"`} {
		if !strings.Contains(html, f) {
			t.Fatalf("formada %s yo'q", f)
		}
	}
}

func TestSearchFormKeepsWindowAndDefaults(t *testing.T) {
	html := render(t, "search.html", searchVM{
		layoutVM: layoutVM{User: &userVM{Name: "Ali"}}, Stations: stations, Today: "2026-12-01",
		Route:  domain.Route{From: defaultFrom, To: defaultTo, Date: "2026-12-01"},
		Window: domain.TimeWindow{From: "08:00", To: "18:00"}, Searched: true, RouteLabel: "Toshkent → Buxoro",
	})
	for _, want := range []string{`name="time_from" value="08:00"`, `name="time_to" value="18:00"`, "08:00–18:00", `value="2900800" selected`, "vaqt oralig"} {
		if !strings.Contains(html, want) {
			t.Errorf("sahifada %q yo'q", want)
		}
	}
}

func TestFilterLabelShowsWindow(t *testing.T) {
	got := filterLabel(domain.Filter{Window: domain.TimeWindow{From: "08:00"}})
	if !strings.Contains(got, "08:00 dan keyin") {
		t.Fatal(got)
	}
	if filterLabel(domain.Filter{}) != "—" {
		t.Fatal("filtrsiz — tire")
	}
}

func TestPageLinks(t *testing.T) {
	desc := func(ls []pageLink) string {
		var b strings.Builder
		for _, l := range ls {
			switch {
			case l.Gap:
				b.WriteString("… ")
			case l.Current:
				b.WriteString("[" + strconv.Itoa(l.N) + "] ")
			default:
				b.WriteString(strconv.Itoa(l.N) + " ")
			}
		}
		return strings.TrimSpace(b.String())
	}
	cases := []struct {
		cur, pages int
		want       string
	}{
		{1, 1, "[1]"},
		{1, 3, "[1] 2 3"},
		{1, 20, "[1] 2 3 … 20"},
		{10, 20, "1 … 8 9 [10] 11 12 … 20"},
		{20, 20, "1 … 18 19 [20]"},
		{4, 20, "1 2 3 [4] 5 6 … 20"},
	}
	for _, c := range cases {
		if got := desc(pageLinks(c.cur, c.pages)); got != c.want {
			t.Errorf("pageLinks(%d,%d) = %q, kutilgan %q", c.cur, c.pages, got, c.want)
		}
	}
}

func TestLogsPageRenders(t *testing.T) {
	p := presenter{stations: fakeStations{}, clock: fixedClock{}}
	lp := usecase.LogPage{
		Watch: domain.Watch{ID: 7, Route: domain.Route{From: "2900000", To: "2900800", Date: "2026-12-05"},
			Filter: domain.Filter{Window: domain.TimeWindow{From: "08:00"}}},
		Page: 2, Pages: 3, Total: 60, PerPage: 25,
		Logs: []domain.CheckLog{
			{ReqTime: time.Date(2026, 11, 24, 15, 0, 0, 0, time.UTC), Duration: 840 * time.Millisecond, OK: true, TrainCount: 2, MatchCount: 1, Notified: true,
				Trains: []domain.TrainSnap{{Number: "764Ф", Label: "Afrosiyob", Departure: "08:37", Matched: true, Cars: []domain.CarSnap{{Label: "O'rindiqli", Seats: 12, Price: 300560}}}}},
			{ReqTime: time.Date(2026, 11, 24, 14, 59, 0, 0, time.UTC), Duration: 15 * time.Second, Error: "tashqi API HTTP 504: Gateway"},
		},
	}
	html := render(t, "logs.html", p.logs(layoutVM{User: &userVM{Name: "Ali"}}, lp))
	for _, want := range []string{"Toshkent → Buxoro", "24.11.2026 15:00:00", "840 ms", "15.0 s", "HTTP 504", "yuborildi",
		"764Ф", "12 joy", `href="?page=1"`, `href="?page=3"`, "26–27", "08:00 dan keyin"} {
		if !strings.Contains(html, want) {
			t.Errorf("logs sahifasida %q yo'q", want)
		}
	}
	if strings.Index(html, "15:00:00") > strings.Index(html, "14:59:00") {
		t.Error("yangi yozuv birinchi bo'lishi kerak")
	}
}

type fakeStations struct{}

func (fakeStations) All() []domain.Station { return stations }
func (fakeStations) Name(code string) string {
	for _, s := range stations {
		if s.Code == code {
			return s.Name
		}
	}
	return code
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 11, 24, 20, 0, 0, 0, time.UTC) }
func (fixedClock) Today() string  { return "2026-11-24" }

func TestDashboardShowsCreationBlock(t *testing.T) {
	html := render(t, "index.html", dashboardVM{
		layoutVM: layoutVM{User: &userVM{Name: "Ali"}}, Stations: stations, Today: "2026-12-01",
		DefaultFrom: defaultFrom, DefaultTo: defaultTo, CreateBlocked: "Kuzatuv yaratish uchun administrator ruxsati kerak.",
		Watches: []watchVM{{ID: 1, RouteLabel: "Toshkent → Buxoro", Active: true, AdminDisabled: true}, {ID: 2, RouteLabel: "A → B", Active: true}},
	})
	if !strings.Contains(html, "mavjud kuzatuvlaringiz ham tekshirilmaydi") {
		t.Fatal("ruxsatsiz foydalanuvchiga mavjud kuzatuvlari ham to'xtashi aytilishi kerak")
	}
	if !strings.Contains(html, "administrator ruxsati kerak") || !strings.Contains(html, "Kuzatuv yaratish</button>") {
		t.Fatal("ogohlantirish ko'rinishi kerak")
	}
	if !strings.Contains(html, `type="submit" disabled>Kuzatuv yaratish`) {
		t.Fatal("tugma o'chirilgan bo'lishi kerak")
	}
	if !strings.Contains(html, "administrator to'xtatgan") {
		t.Fatal("admin to'xtatgan kuzatuv belgilanishi kerak")
	}
	// admin to'xtatgan kuzatuvda "Yoq/To'xtat" tugmasi yo'q, boshqasida bor
	if strings.Contains(html, "/watches/1/toggle") || !strings.Contains(html, "/watches/2/toggle") {
		t.Fatal("toggle faqat admin to'xtatmagan kuzatuvda bo'lishi kerak")
	}
	open := render(t, "index.html", dashboardVM{layoutVM: layoutVM{User: &userVM{Name: "Ali"}}, Stations: stations, Today: "2026-12-01"})
	if strings.Contains(open, "disabled>Kuzatuv yaratish") || strings.Contains(open, `class="warn"`) {
		t.Fatal("ruxsat bo'lsa, forma ochiq bo'lishi kerak")
	}
}

func TestAdminPagesRender(t *testing.T) {
	users := render(t, "admin_users.html", adminUsersVM{
		layoutVM: layoutVM{User: &userVM{Name: "Admin", IsAdmin: true}, AdminTab: "users"},
		Modes:    []modeVM{{Value: "approved", Label: "Faqat ruxsat berilganlar", Current: true}, {Value: "all", Label: "Hamma"}, {Value: "none", Label: "Hech kim"}},
		ModeNote: "izoh", Total: 25, Back: "/admin/users?page=2",
		Users: []adminUserVM{
			{ID: 1, Name: "Ali", Handle: "@ali", TelegramID: "555", Watches: "1 / 2", CanCreate: true},
			{ID: 2, Name: "Vali", TelegramID: "666", Watches: "0 / 0", Disabled: true},
			{ID: 3, Name: "Admin", TelegramID: "900", IsSelf: true, IsAdmin: true},
		},
		Pager: newPager(usecase.Pager{Page: 2, Pages: 3}, nil),
	})
	for _, want := range []string{`class="on">Foydalanuvchilar`, "/admin/settings/creation", `value="all"`, "Ruxsatni ol", "Ruxsat ber",
		"/admin/users/2/disabled", "o'chirilgan", "/admin/watches?user=1", `name="back" value="/admin/users?page=2"`, `href="?page=3"`, `href="?page=1"`} {
		if !strings.Contains(users, want) {
			t.Errorf("admin_users: %q yo'q", want)
		}
	}
	if strings.Contains(users, "/admin/users/3/disabled") {
		t.Error("o'zini o'chirish tugmasi bo'lmasligi kerak")
	}

	watches := render(t, "admin_watches.html", adminWatchesVM{
		layoutVM:  layoutVM{User: &userVM{Name: "Admin", IsAdmin: true}, AdminTab: "watches"},
		OwnerName: "Ali", Total: 2, Back: "/admin/watches?user=1",
		Watches: []adminWatchVM{
			{ID: 10, OwnerID: 1, Owner: "Ali", RouteLabel: "Toshkent → Buxoro", Date: "2026-12-05", Filter: "—", State: "faol", StateClass: "ok",
				Interval: "har 30 soniya", IntervalCustom: true, EveryValue: "30", EveryUnit: "s"},
			{ID: 11, OwnerID: 1, Owner: "Ali", RouteLabel: "A → B", State: "admin to'xtatgan", StateClass: "err", AdminDisabled: true},
		},
		Pager: newPager(usecase.Pager{Page: 1, Pages: 2}, url.Values{"user": {"1"}}),
	})
	for _, want := range []string{`class="on">Kuzatuvlar`, "Hamma kuzatuvlar", "/admin/watches/10/disabled", `name="v" value="1">To'xtat`,
		`name="v" value="0">Yoq`, `href="?page=2&amp;user=1"`, "admin to&#39;xtatgan", "/admin/watches/10/interval", "har 30 soniya", `value="30"`, "Jadvalga", "jadval bo'yicha"} {
		if !strings.Contains(watches, want) {
			t.Errorf("admin_watches: %q yo'q\n%s", want, grepLines(watches, "page=", "to&#39;xtatgan"))
		}
	}
}

func TestAdminBackIsSafe(t *testing.T) {
	mk := func(back string) *http.Request {
		r := httptest.NewRequest("POST", "/x", strings.NewReader(url.Values{"back": {back}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return r
	}
	cases := map[string]string{
		"/admin/users?page=2&msg=eski": "/admin/users?page=2",
		"/admin/watches?user=5":        "/admin/watches?user=5",
		"https://evil.example/admin/x": "/fallback",
		"//evil.example/admin/x":       "/fallback",
		"/search":                      "/fallback",
		"":                             "/fallback",
	}
	for in, want := range cases {
		if got := adminBack(mk(in), "/fallback"); got != want {
			t.Errorf("adminBack(%q) = %q, kutilgan %q", in, got, want)
		}
	}
}

func grepLines(s string, subs ...string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		for _, sub := range subs {
			if strings.Contains(l, sub) {
				out = append(out, strings.TrimSpace(l))
				break
			}
		}
	}
	return strings.Join(out, "\n")
}
