package webui

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/pkg/textfmt"
	"github.com/khasansayliev/watcher/internal/usecase"
)

// View-modellar: shablonlar faqat tayyor qiymatlar bilan ishlaydi, mantiq yo'q.

type userVM struct {
	Name, Email, Avatar string
	IsAdmin             bool
}

type layoutVM struct {
	Title, Flash, FlashKind string
	User                    *userVM
	Paused                  bool
	PausedUntil, Interval   string
}

type loginVM struct {
	Error string
}

type dashboardVM struct {
	layoutVM
	Stations   []domain.Station
	Today      string
	HasTG      bool
	MaxWatches int
	Watches    []watchVM
}

type settingsVM struct {
	layoutVM
	HasTG, TelegramOK bool
	Email, Since      string
}

type watchVM struct {
	ID                                  int64
	RouteLabel, Date, Filter            string
	LastCheck, LastStatus, LastNotified string
	SearchURL                           string
	Active                              bool
	Matches                             int
}

type searchVM struct {
	layoutVM
	Stations   []domain.Station
	Today      string
	Route      domain.Route
	RouteLabel string
	Searched   bool
	Error      string
	Trains     []trainVM
}

type trainVM struct {
	Number, Label, Departure, Arrival, Duration string
	Cars                                        []carVM
}

type carVM struct {
	Label     string
	FreeSeats int
	Price     string
}

type presenter struct {
	stations usecase.StationDirectory
	clock    usecase.Clock
}

func (p presenter) layout(title, flash, kind string, u domain.User, admin bool, st MonitorStatus) layoutVM {
	vm := layoutVM{
		Title: title, Flash: flash, FlashKind: kind,
		User:     &userVM{Name: u.DisplayName(), Email: u.Email, Avatar: u.AvatarURL, IsAdmin: admin},
		Interval: st.Interval().String(),
	}
	if until := st.PausedUntil(); p.clock.Now().Before(until) {
		vm.Paused = true
		vm.PausedUntil = until.In(p.clock.Now().Location()).Format("15:04:05")
	}
	return vm
}

func (p presenter) routeLabel(r domain.Route) string {
	return p.stations.Name(r.From) + " → " + p.stations.Name(r.To)
}

func (p presenter) watches(ws []domain.Watch) []watchVM {
	now := p.clock.Now()
	out := make([]watchVM, 0, len(ws))
	for _, w := range ws {
		vm := watchVM{
			ID:         w.ID,
			RouteLabel: p.routeLabel(w.Route),
			Date:       w.Route.Date,
			Filter:     filterLabel(w.Filter),
			LastCheck:  textfmt.Ago(w.LastCheck, now),
			LastStatus: w.LastStatus,
			Active:     w.Active,
			Matches:    w.LastMatches,
			SearchURL: "/search?" + url.Values{
				"from": {w.Route.From}, "to": {w.Route.To}, "date": {w.Route.Date},
			}.Encode(),
		}
		if !w.LastNotified.IsZero() {
			vm.LastNotified = textfmt.Ago(w.LastNotified, now)
		}
		out = append(out, vm)
	}
	return out
}

func filterLabel(f domain.Filter) string {
	var parts []string
	if f.MaxPrice > 0 {
		parts = append(parts, "≤ "+textfmt.Sum(f.MaxPrice)+" so'm")
	}
	if f.MinSeats > 1 {
		parts = append(parts, "≥ "+strconv.Itoa(f.MinSeats)+" joy")
	}
	if f.CarType != "" {
		parts = append(parts, "«"+f.CarType+"»")
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, ", ")
}

func (p presenter) trains(ts []domain.Train) []trainVM {
	out := make([]trainVM, 0, len(ts))
	for _, t := range ts {
		vm := trainVM{Number: t.Number, Label: t.Label(), Departure: t.Departure, Arrival: t.Arrival, Duration: t.Duration}
		for _, c := range t.Cars {
			cv := carVM{Label: c.Label(), FreeSeats: c.FreeSeats}
			if c.Price > 0 {
				cv.Price = textfmt.Sum(c.Price) + " so'm"
			}
			vm.Cars = append(vm.Cars, cv)
		}
		out = append(out, vm)
	}
	return out
}
