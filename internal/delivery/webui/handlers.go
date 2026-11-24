package webui

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	vm := dashboardVM{
		layoutVM:    s.layout("Kuzatuvlar", r, u),
		DefaultFrom: defaultFrom,
		DefaultTo:   defaultTo,
		Stations:    s.d.Stations.All(),
		Today:       s.d.Clock.Today(),
		MaxWatches:  s.d.MaxWatches,
	}
	if err := s.d.Watches.CanCreate(r.Context(), u.ID); err != nil {
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			vm.CreateBlocked = ve.Msg
		} else {
			slog.Error("yaratish ruxsatini tekshirish", "err", err)
		}
	}
	watches, err := s.d.Watches.List(r.Context(), u.ID)
	if err != nil {
		slog.Error("kuzatuvlar ro'yxati", "err", err)
		vm.Flash, vm.FlashKind = "Kuzatuvlarni yuklab bo'lmadi", "err"
	}
	policy, err := s.d.Polling.Policy(r.Context())
	if err != nil {
		slog.Error("oraliqlar jadvali", "err", err)
		policy = domain.DefaultPollingPolicy()
	}
	vm.Watches = s.p.watches(watches, policy)
	s.render(w, "index.html", vm)
}

// parseWatchForm yaratish va tahrirlash formalarini bir xil o'qiydi. Xato bo'lsa, foydalanuvchiga matn qaytaradi.
func parseWatchForm(r *http.Request) (usecase.CreateWatchInput, string) {
	in := usecase.CreateWatchInput{
		Route: domain.Route{
			From: strings.TrimSpace(r.FormValue("from")),
			To:   strings.TrimSpace(r.FormValue("to")),
			Date: strings.TrimSpace(r.FormValue("date")),
		},
		Filter: domain.Filter{
			CarType: r.FormValue("car_type"),
			Window:  domain.TimeWindow{From: r.FormValue("time_from"), To: r.FormValue("time_to")},
		},
	}
	if v := strings.TrimSpace(r.FormValue("max_price")); v != "" {
		p, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return in, "Maks. narx noto'g'ri"
		}
		in.Filter.MaxPrice = p
	}
	if v := strings.TrimSpace(r.FormValue("min_seats")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return in, "Joylar soni noto'g'ri"
		}
		in.Filter.MinSeats = n
	}
	return in, ""
}

func (s *Server) createWatch(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	in, msg := parseWatchForm(r)
	if msg != "" {
		s.back(w, r, "/", msg, "err")
		return
	}
	if _, err := s.d.Watches.Create(r.Context(), u.ID, in); err != nil {
		s.fail(w, r, "/", err)
		return
	}
	s.back(w, r, "/", "Kuzatuv yaratildi, birinchi tekshiruv boshlandi", "ok")
}

// editWatchPage kuzatuvni tahrirlash formasi (joriy qiymatlar bilan).
func (s *Server) editWatchPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	u := userFrom(r.Context())
	wt, err := s.d.Watches.Get(r.Context(), u.ID, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.fail(w, r, "/", err)
		return
	}
	vm := editVM{
		layoutVM: s.layout("Kuzatuvni tahrirlash", r, u), Stations: s.d.Stations.All(), Today: s.d.Clock.Today(),
		ID: wt.ID, Route: wt.Route, Window: wt.Filter.Window, CarType: wt.Filter.CarType, MinSeats: wt.Filter.MinSeats,
	}
	if wt.Filter.MaxPrice > 0 {
		vm.MaxPrice = strconv.FormatFloat(wt.Filter.MaxPrice, 'f', -1, 64)
	}
	if wt.AdminDisabled {
		vm.Note = "Bu kuzatuv administrator tomonidan to'xtatilgan: tahrirlasangiz ham administrator yoqmaguncha tekshirilmaydi."
	}
	if err := s.d.Watches.CanCreate(r.Context(), u.ID); err != nil {
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			vm.Blocked = ve.Msg
		}
	}
	s.render(w, "edit.html", vm)
}

func (s *Server) editWatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	u := userFrom(r.Context())
	editPath := "/watches/" + strconv.FormatInt(id, 10) + "/edit"
	in, msg := parseWatchForm(r)
	if msg != "" {
		s.back(w, r, editPath, msg, "err")
		return
	}
	if _, err := s.d.Watches.Edit(r.Context(), u.ID, id, in); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.fail(w, r, editPath, err)
		return
	}
	s.back(w, r, "/", "Kuzatuv o'zgartirildi, tekshiruv darhol boshlanadi", "ok")
}

func (s *Server) deleteWatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		s.fail(w, r, "/", domain.ErrNotFound)
		return
	}
	if err := s.d.Watches.Delete(r.Context(), userFrom(r.Context()).ID, id); err != nil {
		s.fail(w, r, "/", err)
		return
	}
	s.back(w, r, "/", "Kuzatuv o'chirildi", "ok")
}

func (s *Server) toggleWatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		s.fail(w, r, "/", domain.ErrNotFound)
		return
	}
	wt, err := s.d.Watches.Toggle(r.Context(), userFrom(r.Context()).ID, id)
	if err != nil {
		s.fail(w, r, "/", err)
		return
	}
	if wt.Active {
		s.back(w, r, "/", "Kuzatuv yoqildi", "ok")
		return
	}
	s.back(w, r, "/", "Kuzatuv to'xtatildi", "ok")
}

func (s *Server) checkWatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		s.fail(w, r, "/", domain.ErrNotFound)
		return
	}
	if err := s.d.Watches.RequestCheck(r.Context(), userFrom(r.Context()).ID, id); err != nil {
		s.fail(w, r, "/", err)
		return
	}
	s.back(w, r, "/", "Tekshiruv navbatga qo'yildi — bir necha soniyadan keyin sahifani yangilang", "ok")
}

// watchLogs kuzatuvning tekshiruv loglarini req_time bo'yicha kamayish tartibida, sahifalab ko'rsatadi.
func (s *Server) watchLogs(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	u := userFrom(r.Context())
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	lp, err := s.d.Watches.Logs(r.Context(), u.ID, id, page)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.fail(w, r, "/", err)
		return
	}
	s.render(w, "logs.html", s.p.logs(s.layout("Loglar", r, u), lp))
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	q := r.URL.Query()
	route := domain.Route{From: q.Get("from"), To: q.Get("to"), Date: q.Get("date")}
	window := domain.TimeWindow{From: strings.TrimSpace(q.Get("time_from")), To: strings.TrimSpace(q.Get("time_to"))}
	vm := searchVM{layoutVM: s.layout("Qidiruv", r, u), Stations: s.d.Stations.All(), Today: s.d.Clock.Today(), Route: route, Window: window}
	if vm.Route.Date == "" {
		vm.Route.Date = vm.Today
	}
	// Forma uchun standart yo'nalish; qidiruv faqat so'rovda aniq berilgan qiymatlar bilan ishlaydi.
	if vm.Route.From == "" {
		vm.Route.From = defaultFrom
	}
	if vm.Route.To == "" {
		vm.Route.To = defaultTo
	}
	if route.From != "" && route.To != "" && route.Date != "" {
		vm.Searched = true
		vm.RouteLabel = s.p.routeLabel(route)
		if err := window.Validate(); err != nil {
			vm.Error = userMessage(err)
		} else if trains, err := s.d.Search.Search(r.Context(), u.ID, route); err != nil {
			vm.Error = userMessage(err)
		} else {
			vm.Trains = s.p.trains(window.FilterTrains(trains))
		}
	}
	s.render(w, "search.html", vm)
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	s.render(w, "settings.html", settingsVM{
		layoutVM:   s.layout("Sozlamalar", r, u),
		TelegramOK: s.d.TelegramOK,
		Username:   u.Username,
		Since:      u.CreatedAt.In(s.d.Clock.Now().Location()).Format("02.01.2006"),
	})
}

func (s *Server) telegramTest(w http.ResponseWriter, r *http.Request) {
	if err := s.d.Telegram.SendTest(r.Context(), userFrom(r.Context()).ID); err != nil {
		s.fail(w, r, "/settings", err)
		return
	}
	s.back(w, r, "/settings", "Test xabar yuborildi", "ok")
}

// lastResponse — faqat administratorlar uchun (API formatini moslashtirishda kerak).
func (s *Server) lastResponse(w http.ResponseWriter, r *http.Request) {
	if !userFrom(r.Context()).IsAdmin(s.d.AdminIDs) {
		http.NotFound(w, r)
		return
	}
	raw := s.d.Raw.LastRaw()
	if len(raw) == 0 {
		http.Error(w, "Hali so'rov yuborilmagan", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(raw)
}

// ---- yordamchilar ----

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func (s *Server) layout(title string, r *http.Request, u domain.User) layoutVM {
	return s.p.layout(title, r.URL.Query().Get("msg"), r.URL.Query().Get("kind"), u, u.IsAdmin(s.d.AdminIDs), s.d.Status)
}

func (s *Server) render(w http.ResponseWriter, name string, vm any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, vm); err != nil {
		slog.Error("template", "name", name, "err", err)
	}
}

func (s *Server) back(w http.ResponseWriter, r *http.Request, path, msg, kind string) {
	http.Redirect(w, r, path+"?kind="+kind+"&msg="+urlEscape(msg), http.StatusSeeOther)
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, path string, err error) {
	s.back(w, r, path, userMessage(err), "err")
}

// userMessage xatoni foydalanuvchiga ko'rsatiladigan matnga aylantiradi (ichki tafsilotlarsiz).
func userMessage(err error) string {
	var (
		ve *domain.ValidationError
		pe *usecase.PausedError
		te *usecase.ThrottledError
		ue *usecase.UpstreamError
	)
	switch {
	case errors.As(err, &ve):
		return ve.Msg
	case errors.As(err, &pe), errors.As(err, &te):
		return err.Error()
	case errors.Is(err, domain.ErrForbidden):
		return "Bu amal faqat administratorlar uchun"
	case errors.Is(err, domain.ErrNotFound):
		return "Topilmadi"
	case errors.As(err, &ue):
		return "Temir yo'l sayti xato qaytardi (HTTP " + strconv.Itoa(ue.Status) + "). Keyinroq urinib ko'ring"
	default:
		slog.Error("ichki xato", "err", err)
		return "Ichki xato yuz berdi. Keyinroq urinib ko'ring"
	}
}

func urlEscape(s string) string { return url.QueryEscape(s) }
