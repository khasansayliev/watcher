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
		layoutVM:   s.layout("Kuzatuvlar", r, u),
		Stations:   s.d.Stations.All(),
		Today:      s.d.Clock.Today(),
		HasTG:      u.HasTelegram(),
		MaxWatches: s.d.MaxWatches,
	}
	watches, err := s.d.Watches.List(r.Context(), u.ID)
	if err != nil {
		slog.Error("kuzatuvlar ro'yxati", "err", err)
		vm.Flash, vm.FlashKind = "Kuzatuvlarni yuklab bo'lmadi", "err"
	}
	vm.Watches = s.p.watches(watches)
	s.render(w, "index.html", vm)
}

func (s *Server) createWatch(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	in := usecase.CreateWatchInput{
		Route: domain.Route{
			From: strings.TrimSpace(r.FormValue("from")),
			To:   strings.TrimSpace(r.FormValue("to")),
			Date: strings.TrimSpace(r.FormValue("date")),
		},
		Filter: domain.Filter{CarType: r.FormValue("car_type")},
	}
	if v := r.FormValue("max_price"); v != "" {
		p, err := strconv.ParseFloat(v, 64)
		if err != nil {
			s.back(w, r, "/", "Maks. narx noto'g'ri", "err")
			return
		}
		in.Filter.MaxPrice = p
	}
	if v := r.FormValue("min_seats"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			s.back(w, r, "/", "Joylar soni noto'g'ri", "err")
			return
		}
		in.Filter.MinSeats = n
	}
	if _, err := s.d.Watches.Create(r.Context(), u.ID, in); err != nil {
		s.fail(w, r, "/", err)
		return
	}
	s.back(w, r, "/", "Kuzatuv yaratildi, birinchi tekshiruv boshlandi", "ok")
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

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	q := r.URL.Query()
	route := domain.Route{From: q.Get("from"), To: q.Get("to"), Date: q.Get("date")}
	vm := searchVM{layoutVM: s.layout("Qidiruv", r, u), Stations: s.d.Stations.All(), Today: s.d.Clock.Today(), Route: route}
	if vm.Route.Date == "" {
		vm.Route.Date = vm.Today
	}
	if route.From != "" && route.To != "" && route.Date != "" {
		vm.Searched = true
		vm.RouteLabel = s.p.routeLabel(route)
		trains, err := s.d.Search.Search(r.Context(), u.ID, route)
		if err != nil {
			vm.Error = userMessage(err)
		} else {
			vm.Trains = s.p.trains(trains)
		}
	}
	s.render(w, "search.html", vm)
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	s.render(w, "settings.html", settingsVM{
		layoutVM:   s.layout("Sozlamalar", r, u),
		HasTG:      u.HasTelegram(),
		TelegramOK: s.d.TelegramOK,
		Email:      u.Email,
		Since:      u.CreatedAt.In(s.d.Clock.Now().Location()).Format("02.01.2006"),
	})
}

func (s *Server) telegramLink(w http.ResponseWriter, r *http.Request) {
	link, err := s.d.Telegram.StartLink(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		s.fail(w, r, "/settings", err)
		return
	}
	http.Redirect(w, r, link, http.StatusSeeOther)
}

func (s *Server) telegramUnlink(w http.ResponseWriter, r *http.Request) {
	if err := s.d.Telegram.Unlink(r.Context(), userFrom(r.Context()).ID); err != nil {
		s.fail(w, r, "/settings", err)
		return
	}
	s.back(w, r, "/settings", "Telegram uzildi", "ok")
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
	if !userFrom(r.Context()).IsAdmin(s.d.AdminEmails) {
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
	return s.p.layout(title, r.URL.Query().Get("msg"), r.URL.Query().Get("kind"), u, u.IsAdmin(s.d.AdminEmails), s.d.Status)
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
