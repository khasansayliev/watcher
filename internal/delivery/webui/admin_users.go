package webui

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/pkg/textfmt"
	"github.com/khasansayliev/watcher/internal/usecase"
)

// ---- view-modellar ----

type pagerVM struct {
	Pages              int
	PrevHref, NextHref string // bo'sh = yo'q
	Links              []pageLink
}

// newPager sahifa havolalarini tayyorlaydi; extra — har bir havolada saqlanadigan parametrlar (masalan user=5).
// Havolalar Go'da yig'iladi: shablonda "&" va "=" ni qo'shib bo'lmaydi (URL kontekstida kodlanib qoladi).
func newPager(p usecase.Pager, extra url.Values) pagerVM {
	href := func(n int) string {
		q := url.Values{}
		for k, v := range extra {
			q[k] = v
		}
		q.Set("page", strconv.Itoa(n))
		return "?" + q.Encode()
	}
	vm := pagerVM{Pages: p.Pages, Links: pageLinks(p.Page, p.Pages)}
	for i := range vm.Links {
		if !vm.Links[i].Gap {
			vm.Links[i].Href = href(vm.Links[i].N)
		}
	}
	if p.Page > 1 {
		vm.PrevHref = href(p.Page - 1)
	}
	if p.Page < p.Pages {
		vm.NextHref = href(p.Page + 1)
	}
	return vm
}

type modeVM struct {
	Value, Label, Hint string
	Current            bool
}

type adminUserVM struct {
	ID                       int64
	Name, Handle, TelegramID string
	Created, LastLogin       string
	Watches                  string // "faol / jami"
	CanCreate, Disabled      bool
	IsSelf, IsAdmin          bool
}

type adminUsersVM struct {
	layoutVM
	Modes    []modeVM
	ModeNote string
	Users    []adminUserVM
	Total    int
	Pager    pagerVM
	Back     string
}

type adminWatchVM struct {
	ID                       int64
	OwnerID                  int64
	Owner                    string
	RouteLabel, Date, Filter string
	State, StateClass        string
	LastCheck, LastStatus    string
	Matches                  int
	AdminDisabled            bool
}

type adminWatchesVM struct {
	layoutVM
	OwnerName string // bo'sh emas bo'lsa — bitta foydalanuvchi bo'yicha filtr
	Watches   []adminWatchVM
	Total     int
	Pager     pagerVM
	Back      string
}

// ---- yordamchilar ----

// adminOnly admin bo'lmaganlarga sahifa mavjudligini ham bildirmaydi (404).
func (s *Server) adminOnly(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
	u := userFrom(r.Context())
	if !u.IsAdmin(s.d.AdminIDs) {
		http.NotFound(w, r)
		return u, false
	}
	return u, true
}

// adminBack formadagi "back" ni faqat /admin/ ichidagi nisbiy manzil bo'lsa qabul qiladi (open redirect yo'q).
func adminBack(r *http.Request, fallback string) string {
	b := r.FormValue("back")
	u, err := url.Parse(b)
	if err != nil || u.Scheme != "" || u.Host != "" || !strings.HasPrefix(u.Path, "/admin/") || strings.HasPrefix(b, "//") {
		return fallback
	}
	q := u.Query()
	q.Del("msg")
	q.Del("kind")
	u.RawQuery = q.Encode()
	return u.String()
}

func (s *Server) backTo(w http.ResponseWriter, r *http.Request, target, msg, kind string) {
	u, _ := url.Parse(target)
	q := u.Query()
	q.Set("kind", kind)
	q.Set("msg", msg)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}

func (s *Server) adminLayout(title, tab string, r *http.Request, u domain.User) layoutVM {
	l := s.layout(title, r, u)
	l.AdminTab = tab
	return l
}

func queryInt(r *http.Request, key string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(key))
	return n
}

func userHandle(u domain.User) string {
	if u.Username != "" {
		return "@" + u.Username
	}
	return ""
}

// ---- foydalanuvchilar ----

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request) {
	u, ok := s.adminOnly(w, r)
	if !ok {
		return
	}
	pg, err := s.d.Admin.Users(r.Context(), u, queryInt(r, "page"))
	if err != nil {
		s.fail(w, r, "/", err)
		return
	}
	loc := s.d.Clock.Now().Location()
	vm := adminUsersVM{
		layoutVM: s.adminLayout("Admin · Foydalanuvchilar", "users", r, u),
		Total:    pg.Total, Pager: newPager(pg.Pager, nil), Back: r.URL.RequestURI(),
	}
	cur := pg.Settings.CreationMode
	for _, m := range []modeVM{
		{Value: string(domain.CreationApproved), Label: "Faqat ruxsat berilganlar", Hint: "yangi foydalanuvchi ruxsatsiz boshlaydi; ruxsatni quyida har bir foydalanuvchiga siz berasiz. Ruxsati yo'qlarning kuzatuvlari ham tekshirilmaydi"},
		{Value: string(domain.CreationAll), Label: "Hamma", Hint: "shaxsiy ruxsatlardan qat'i nazar, hamma kuzatuv yarata oladi"},
		{Value: string(domain.CreationNone), Label: "Hech kim", Hint: "yangi kuzatuv yaratish ham, boshqalarning mavjud kuzatuvlarini tekshirish ham to'xtaydi (adminlardan tashqari)"},
	} {
		m.Current = string(cur) == m.Value
		if m.Current {
			vm.ModeNote = m.Hint
		}
		vm.Modes = append(vm.Modes, m)
	}
	for _, x := range pg.Users {
		row := adminUserVM{
			ID: x.ID, Name: x.DisplayName(), Handle: userHandle(x.User), TelegramID: x.TelegramChatID,
			Created:   x.CreatedAt.In(loc).Format("02.01.2006"),
			Watches:   strconv.Itoa(x.ActiveWatches) + " / " + strconv.Itoa(x.TotalWatches),
			CanCreate: x.CanCreate, Disabled: x.Disabled,
			IsSelf: x.ID == u.ID, IsAdmin: x.User.IsAdmin(s.d.AdminIDs),
		}
		if !x.LastLoginAt.IsZero() {
			row.LastLogin = textfmt.Ago(x.LastLoginAt, s.d.Clock.Now())
		}
		vm.Users = append(vm.Users, row)
	}
	s.render(w, "admin_users.html", vm)
}

func (s *Server) adminUserCreation(w http.ResponseWriter, r *http.Request) {
	u, ok := s.adminOnly(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	back := adminBack(r, "/admin/users")
	allow := r.FormValue("v") == "1"
	if err := s.d.Admin.SetCanCreate(r.Context(), u, id, allow); err != nil {
		s.backTo(w, r, back, userMessage(err), "err")
		return
	}
	if allow {
		s.backTo(w, r, back, "Kuzatuv yaratishga ruxsat berildi", "ok")
		return
	}
	s.backTo(w, r, back, "Ruxsat olindi: yangi kuzatuv yarata olmaydi va mavjud kuzatuvlari ham tekshirilmaydi", "ok")
}

func (s *Server) adminUserDisabled(w http.ResponseWriter, r *http.Request) {
	u, ok := s.adminOnly(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	back := adminBack(r, "/admin/users")
	disable := r.FormValue("v") == "1"
	if err := s.d.Admin.SetUserDisabled(r.Context(), u, id, disable); err != nil {
		s.backTo(w, r, back, userMessage(err), "err")
		return
	}
	if disable {
		s.backTo(w, r, back, "Foydalanuvchi o'chirildi: kira olmaydi, kuzatuvlari tekshirilmaydi", "ok")
		return
	}
	s.backTo(w, r, back, "Foydalanuvchi qayta yoqildi", "ok")
}

func (s *Server) adminCreationMode(w http.ResponseWriter, r *http.Request) {
	u, ok := s.adminOnly(w, r)
	if !ok {
		return
	}
	back := adminBack(r, "/admin/users")
	if err := s.d.Admin.SetCreationMode(r.Context(), u, domain.CreationMode(r.FormValue("mode"))); err != nil {
		s.backTo(w, r, back, userMessage(err), "err")
		return
	}
	s.backTo(w, r, back, "Kuzatuv yaratish rejimi saqlandi", "ok")
}

// ---- kuzatuvlar ----

func (s *Server) adminWatches(w http.ResponseWriter, r *http.Request) {
	u, ok := s.adminOnly(w, r)
	if !ok {
		return
	}
	userID := int64(queryInt(r, "user"))
	pg, err := s.d.Admin.Watches(r.Context(), u, userID, queryInt(r, "page"))
	if err != nil {
		s.fail(w, r, "/admin/users", err)
		return
	}
	var extra url.Values
	vm := adminWatchesVM{layoutVM: s.adminLayout("Admin · Kuzatuvlar", "watches", r, u), Total: pg.Total, Back: r.URL.RequestURI()}
	if pg.Owner != nil {
		vm.OwnerName = pg.Owner.DisplayName()
		extra = url.Values{"user": {strconv.FormatInt(userID, 10)}}
	}
	vm.Pager = newPager(pg.Pager, extra)
	now := s.d.Clock.Now()
	for _, x := range pg.Watches {
		row := adminWatchVM{
			ID: x.ID, OwnerID: x.UserID, Owner: x.Owner.DisplayName(),
			RouteLabel: s.p.routeLabel(x.Route), Date: x.Route.Date, Filter: filterLabel(x.Filter),
			LastCheck: textfmt.Ago(x.LastCheck, now), LastStatus: x.LastStatus, Matches: x.LastMatches,
			AdminDisabled: x.AdminDisabled,
		}
		switch {
		case x.AdminDisabled:
			row.State, row.StateClass = "admin to'xtatgan", "err"
		case x.Owner.Disabled:
			row.State, row.StateClass = "egasi o'chirilgan", "err"
		case x.NoPermission:
			row.State, row.StateClass = "ruxsat yo'q", "err"
		case !x.Active:
			row.State, row.StateClass = "to'xtatilgan", ""
		default:
			row.State, row.StateClass = "faol", "ok"
		}
		vm.Watches = append(vm.Watches, row)
	}
	s.render(w, "admin_watches.html", vm)
}

func (s *Server) adminWatchDisabled(w http.ResponseWriter, r *http.Request) {
	u, ok := s.adminOnly(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	back := adminBack(r, "/admin/watches")
	disable := r.FormValue("v") == "1"
	if err := s.d.Admin.SetWatchDisabled(r.Context(), u, id, disable); err != nil {
		s.backTo(w, r, back, userMessage(err), "err")
		return
	}
	if disable {
		s.backTo(w, r, back, "Kuzatuv to'xtatildi — foydalanuvchi uni o'zi yoqa olmaydi", "ok")
		return
	}
	s.backTo(w, r, back, "Kuzatuv qayta yoqildi", "ok")
}
