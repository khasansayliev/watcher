package webui

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/khasansayliev/railbot/internal/domain"
	"github.com/khasansayliev/railbot/internal/usecase"
)

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	watches, err := s.d.Watches.List(r.Context())
	vm := indexVM{layoutVM: s.layout("Kuzatuvlar", r), Stations: s.d.Stations.All(), Today: s.d.Clock.Today(), DefaultChatID: s.d.DefaultChatID}
	if err != nil {
		vm.Flash, vm.FlashKind = err.Error(), "err"
	}
	vm.Watches = s.p.watches(watches)
	s.render(w, "index.html", vm)
}

func (s *Server) createWatch(w http.ResponseWriter, r *http.Request) {
	in := usecase.CreateWatchInput{
		Route: domain.Route{
			From: strings.TrimSpace(r.FormValue("from")),
			To:   strings.TrimSpace(r.FormValue("to")),
			Date: strings.TrimSpace(r.FormValue("date")),
		},
		ChatID: r.FormValue("chat_id"),
		Filter: domain.Filter{CarType: r.FormValue("car_type")},
	}
	if v := r.FormValue("max_price"); v != "" {
		p, err := strconv.ParseFloat(v, 64)
		if err != nil {
			s.back(w, r, "Maks. narx noto'g'ri", "err")
			return
		}
		in.Filter.MaxPrice = p
	}
	if v := r.FormValue("min_seats"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			s.back(w, r, "Joylar soni noto'g'ri", "err")
			return
		}
		in.Filter.MinSeats = n
	}
	if _, err := s.d.Watches.Create(r.Context(), in); err != nil {
		s.fail(w, r, err)
		return
	}
	s.back(w, r, "Kuzatuv yaratildi, birinchi tekshiruv boshlandi", "ok")
}

func (s *Server) deleteWatch(w http.ResponseWriter, r *http.Request) {
	if err := s.d.Watches.Delete(r.Context(), r.PathValue("id")); err != nil {
		s.fail(w, r, err)
		return
	}
	s.back(w, r, "Kuzatuv o'chirildi", "ok")
}

func (s *Server) toggleWatch(w http.ResponseWriter, r *http.Request) {
	wt, err := s.d.Watches.Toggle(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if wt.Active {
		s.back(w, r, "Kuzatuv yoqildi", "ok")
		return
	}
	s.back(w, r, "Kuzatuv to'xtatildi", "ok")
}

func (s *Server) checkWatch(w http.ResponseWriter, r *http.Request) {
	if err := s.d.Watches.RequestCheck(r.Context(), r.PathValue("id")); err != nil {
		s.fail(w, r, err)
		return
	}
	s.back(w, r, "Tekshiruv navbatga qo'yildi — bir necha soniyadan keyin sahifani yangilang", "ok")
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	route := domain.Route{From: q.Get("from"), To: q.Get("to"), Date: q.Get("date")}
	vm := searchVM{layoutVM: s.layout("Qidiruv", r), Stations: s.d.Stations.All(), Today: s.d.Clock.Today(), Route: route}
	if vm.Route.Date == "" {
		vm.Route.Date = vm.Today
	}
	if route.From != "" && route.To != "" && route.Date != "" {
		vm.Searched = true
		vm.RouteLabel = s.p.routeLabel(route)
		trains, err := s.d.Search.Search(r.Context(), route)
		if err != nil {
			vm.Error = err.Error()
		} else {
			vm.Trains = s.p.trains(trains)
		}
	}
	s.render(w, "search.html", vm)
}

func (s *Server) testTelegram(w http.ResponseWriter, r *http.Request) {
	if err := s.d.Watches.SendTest(r.Context(), r.FormValue("chat_id")); err != nil {
		s.fail(w, r, err)
		return
	}
	s.back(w, r, "Test xabar yuborildi", "ok")
}

func (s *Server) lastResponse(w http.ResponseWriter, _ *http.Request) {
	raw := s.d.Raw.LastRaw()
	if len(raw) == 0 {
		http.Error(w, "Hali so'rov yuborilmagan", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(raw)
}

// ---- yordamchilar ----

func (s *Server) layout(title string, r *http.Request) layoutVM {
	return s.p.layout(title, r.URL.Query().Get("msg"), r.URL.Query().Get("kind"), s.d.TelegramOK, s.d.Status)
}

func (s *Server) render(w http.ResponseWriter, name string, vm any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, vm); err != nil {
		slog.Error("template", "name", name, "err", err)
	}
}

func (s *Server) back(w http.ResponseWriter, r *http.Request, msg, kind string) {
	http.Redirect(w, r, "/?kind="+kind+"&msg="+url.QueryEscape(msg), http.StatusSeeOther)
}

// fail domen/usecase xatosini foydalanuvchiga tushunarli xabarga aylantiradi.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var ve *domain.ValidationError
	switch {
	case errors.As(err, &ve):
		s.back(w, r, ve.Msg, "err")
	case errors.Is(err, domain.ErrNotFound):
		s.back(w, r, err.Error(), "err")
	default:
		slog.Error("so'rov xatosi", "path", r.URL.Path, "err", err)
		s.back(w, r, "Xato: "+err.Error(), "err")
	}
}
