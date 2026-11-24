package webui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/khasansayliev/watcher/internal/domain"
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
