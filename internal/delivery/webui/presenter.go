package webui

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/pkg/textfmt"
	"github.com/khasansayliev/watcher/internal/usecase"
)

// View-modellar: shablonlar faqat tayyor qiymatlar bilan ishlaydi, mantiq yo'q.

type userVM struct {
	Name    string
	IsAdmin bool
}

type layoutVM struct {
	Title, Flash, FlashKind string
	User                    *userVM
	Paused                  bool
	PausedUntil             string
	AdminTab                string // admin sahifalarida faol bo'lim: users | watches | polling
}

type loginVM struct {
	Error string
	// CodeBotURL — botni "/start login" bilan ochadigan havola (Telegram'siz qurilma uchun kod olish); bo'sh bo'lishi mumkin.
	CodeBotURL string
}

type loginWaitVM struct {
	BotURL string
}

// Forma uchun standart yo'nalish: Toshkent → Buxoro (kodlar stations.json dagi bilan bir xil).
const (
	defaultFrom = "2900000"
	defaultTo   = "2900800"
)

type dashboardVM struct {
	layoutVM
	DefaultFrom, DefaultTo string
	CreateBlocked          string // bo'sh emas bo'lsa — kuzatuv yaratib bo'lmaydi, sababi shu
	Stations               []domain.Station
	Today                  string
	MaxWatches             int
	Watches                []watchVM
}

type settingsVM struct {
	layoutVM
	TelegramOK      bool
	Username, Since string
}

type watchVM struct {
	ID                                  int64
	RouteLabel, Date, Filter            string
	LastCheck, LastStatus, LastNotified string
	SearchURL, Interval                 string
	Active                              bool
	AdminDisabled                       bool
	Matches                             int
}

type searchVM struct {
	layoutVM
	Window     domain.TimeWindow
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
		User: &userVM{Name: u.DisplayName(), IsAdmin: admin},
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

func (p presenter) watches(ws []domain.Watch, policy domain.PollingPolicy) []watchVM {
	now, today := p.clock.Now(), p.clock.Today()
	out := make([]watchVM, 0, len(ws))
	for _, w := range ws {
		vm := watchVM{
			ID:            w.ID,
			RouteLabel:    p.routeLabel(w.Route),
			Date:          w.Route.Date,
			Filter:        filterLabel(w.Filter),
			LastCheck:     textfmt.Ago(w.LastCheck, now),
			LastStatus:    w.LastStatus,
			Active:        w.Active,
			AdminDisabled: w.AdminDisabled,
			Matches:       w.LastMatches,
			Interval:      "har " + textfmt.Interval(w.IntervalIn(policy, today)),
			SearchURL:     "/search?" + searchQuery(w).Encode(),
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
	if l := f.Window.Label(); l != "" {
		parts = append(parts, "🕒 "+l)
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

// searchQuery kuzatuv parametrlari bilan "Ko'rish" havolasi (vaqt oralig'i ham o'tadi).
func searchQuery(w domain.Watch) url.Values {
	q := url.Values{"from": {w.Route.From}, "to": {w.Route.To}, "date": {w.Route.Date}}
	if w.Filter.Window.From != "" {
		q.Set("time_from", w.Filter.Window.From)
	}
	if w.Filter.Window.To != "" {
		q.Set("time_to", w.Filter.Window.To)
	}
	return q
}

type logVM struct {
	Time, Duration         string
	OK, Notified           bool
	Manual                 bool
	Error, NotifyErr       string
	TrainCount, MatchCount int
	Trains                 []logTrainVM
}

type logTrainVM struct {
	Number, Label, Departure, Arrival string
	Matched                           bool
	Cars                              []carVM
}

type pageLink struct {
	N       int
	Current bool
	Gap     bool   // "…"
	Href    string // admin pagerida to'ldiriladi
}

type logsVM struct {
	layoutVM
	WatchID            int64
	RouteLabel, Date   string
	Filter             string
	Logs               []logVM
	Page, Pages, Total int
	From, To           int // ko'rsatilgan yozuvlar oralig'i (1 dan)
	Prev, Next         int // 0 = yo'q
	Links              []pageLink
}

func (p presenter) logs(l layoutVM, lp usecase.LogPage) logsVM {
	loc := p.clock.Now().Location()
	vm := logsVM{
		layoutVM: l, WatchID: lp.Watch.ID, RouteLabel: p.routeLabel(lp.Watch.Route), Date: lp.Watch.Route.Date,
		Filter: filterLabel(lp.Watch.Filter), Page: lp.Page, Pages: lp.Pages, Total: lp.Total,
	}
	for _, x := range lp.Logs {
		lv := logVM{
			Time: x.ReqTime.In(loc).Format("02.01.2006 15:04:05"), Duration: durationLabel(x.Duration),
			OK: x.OK, Notified: x.Notified, Manual: x.Manual, Error: x.Error, NotifyErr: x.NotifyErr,
			TrainCount: x.TrainCount, MatchCount: x.MatchCount,
		}
		for _, t := range x.Trains {
			tv := logTrainVM{Number: t.Number, Label: t.Label, Departure: t.Departure, Arrival: t.Arrival, Matched: t.Matched}
			for _, c := range t.Cars {
				cv := carVM{Label: c.Label, FreeSeats: c.Seats}
				if c.Price > 0 {
					cv.Price = textfmt.Sum(c.Price) + " so'm"
				}
				tv.Cars = append(tv.Cars, cv)
			}
			lv.Trains = append(lv.Trains, tv)
		}
		vm.Logs = append(vm.Logs, lv)
	}
	if lp.Total > 0 {
		vm.From = (lp.Page-1)*lp.PerPage + 1
		vm.To = vm.From + len(lp.Logs) - 1
	}
	if lp.Page > 1 {
		vm.Prev = lp.Page - 1
	}
	if lp.Page < lp.Pages {
		vm.Next = lp.Page + 1
	}
	vm.Links = pageLinks(lp.Page, lp.Pages)
	return vm
}

// pageLinks birinchi, oxirgi va joriy atrofidagi sahifalarni qaytaradi, orasiga "…" qo'yadi.
func pageLinks(cur, pages int) []pageLink {
	var out []pageLink
	last := 0
	for n := 1; n <= pages; n++ {
		if n != 1 && n != pages && (n < cur-2 || n > cur+2) {
			continue
		}
		if last != 0 && n-last > 1 {
			out = append(out, pageLink{Gap: true})
		}
		out = append(out, pageLink{N: n, Current: n == cur})
		last = n
	}
	return out
}

func durationLabel(d time.Duration) string {
	if d < time.Second {
		return strconv.FormatInt(d.Milliseconds(), 10) + " ms"
	}
	return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + " s"
}

type editVM struct {
	layoutVM
	Stations []domain.Station
	Today    string
	ID       int64
	Route    domain.Route
	Window   domain.TimeWindow
	MaxPrice string
	MinSeats int
	CarType  string
	Blocked  string // bo'sh emas bo'lsa — tahrirlash mumkin emas (ruxsat yo'q), sababi shu
	Note     string
}
