package main

import (
	"context"
	"crypto/subtle"
	"embed"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed templates/*.html
var tmplFS embed.FS

// Qo'lda qidiruvlar orasidagi minimal pauza (saytga yuklama bermaslik uchun).
const manualSearchGap = 10 * time.Second

type Web struct {
	cfg      Config
	store    *Store
	watcher  *Watcher
	rc       *RailwayClient
	tg       *Telegram
	st       *Stations
	tmpl     *template.Template
	searchMu sync.Mutex
	lastSrch time.Time
}

type pageData struct {
	Title, Flash, FlashKind string
	Stations                []Station
	Today                   string
	TelegramOK              bool
	DefaultChatID           string
	Interval                time.Duration
	Paused                  bool
	PausedUntil             string

	Watches []Watch

	From, To, Date string
	Searched       bool
	SearchErr      string
	Trains         []Train
}

func NewWeb(cfg Config, store *Store, w *Watcher, rc *RailwayClient, tg *Telegram, st *Stations) *Web {
	funcs := template.FuncMap{
		"station":    st.Name,
		"price":      formatPrice,
		"ago":        ago,
		"trainLabel": trainLabel,
		"carLabel":   carLabel,
	}
	t := template.Must(template.New("").Funcs(funcs).ParseFS(tmplFS, "templates/*.html"))
	return &Web{cfg: cfg, store: store, watcher: w, rc: rc, tg: tg, st: st, tmpl: t}
}

func (h *Web) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.index)
	mux.HandleFunc("POST /watches", h.createWatch)
	mux.HandleFunc("POST /watches/{id}/delete", h.deleteWatch)
	mux.HandleFunc("POST /watches/{id}/toggle", h.toggleWatch)
	mux.HandleFunc("POST /watches/{id}/check", h.checkWatch)
	mux.HandleFunc("GET /search", h.search)
	mux.HandleFunc("POST /telegram/test", h.testTelegram)
	mux.HandleFunc("GET /debug/last-response", h.lastResponse)
	return h.guard(mux)
}

// guard: ixtiyoriy Basic Auth + boshqa saytlardan POST'ni bloklash.
func (h *Web) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.cfg.UIPassword != "" {
			u, p, ok := r.BasicAuth()
			if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(h.cfg.UIUser)) != 1 ||
				subtle.ConstantTimeCompare([]byte(p), []byte(h.cfg.UIPassword)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="railbot"`)
				http.Error(w, "Kirish taqiqlangan", http.StatusUnauthorized)
				return
			}
		}
		if r.Method == http.MethodPost {
			if o := r.Header.Get("Origin"); o != "" {
				if ou, err := url.Parse(o); err != nil || ou.Host != r.Host {
					http.Error(w, "noto'g'ri origin", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Web) base(title string, r *http.Request) pageData {
	d := pageData{
		Title:         title,
		Stations:      h.st.List,
		Today:         h.watcher.Today(),
		TelegramOK:    h.tg.Enabled(),
		DefaultChatID: h.cfg.DefaultChatID,
		Interval:      h.watcher.Interval(),
		Flash:         r.URL.Query().Get("msg"),
		FlashKind:     r.URL.Query().Get("kind"),
	}
	if pu := h.watcher.PausedUntil(); time.Now().Before(pu) {
		d.Paused = true
		d.PausedUntil = pu.In(h.watcher.loc).Format("15:04:05")
	}
	return d
}

func (h *Web) render(w http.ResponseWriter, name string, d pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, name, d); err != nil {
		log.Printf("template %s: %v", name, err)
	}
}

func redirect(w http.ResponseWriter, r *http.Request, msg, kind string) {
	http.Redirect(w, r, "/?kind="+kind+"&msg="+url.QueryEscape(msg), http.StatusSeeOther)
}

func (h *Web) index(w http.ResponseWriter, r *http.Request) {
	d := h.base("Kuzatuvlar", r)
	d.Watches = h.store.List()
	h.render(w, "index.html", d)
}

func (h *Web) createWatch(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		redirect(w, r, "Forma xatosi", "err")
		return
	}
	wt := Watch{
		From:    strings.TrimSpace(r.FormValue("from")),
		To:      strings.TrimSpace(r.FormValue("to")),
		Date:    strings.TrimSpace(r.FormValue("date")),
		ChatID:  strings.TrimSpace(r.FormValue("chat_id")),
		CarType: strings.TrimSpace(r.FormValue("car_type")),
		Active:  true,
	}
	if wt.From == "" || wt.To == "" || wt.From == wt.To {
		redirect(w, r, "Qayerdan va qayerga har xil bo'lishi kerak", "err")
		return
	}
	if _, err := time.Parse("2006-01-02", wt.Date); err != nil || wt.Date < h.watcher.Today() {
		redirect(w, r, "Sana noto'g'ri yoki o'tib ketgan", "err")
		return
	}
	if wt.ChatID == "" {
		redirect(w, r, "Telegram chat ID kiriting", "err")
		return
	}
	if v := r.FormValue("max_price"); v != "" {
		p, err := strconv.ParseFloat(v, 64)
		if err != nil || p < 0 {
			redirect(w, r, "Maks. narx noto'g'ri", "err")
			return
		}
		wt.MaxPrice = p
	}
	if v := r.FormValue("min_seats"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 50 {
			redirect(w, r, "Joylar soni 1–50 oralig'ida bo'lsin", "err")
			return
		}
		wt.MinSeats = n
	}
	saved, err := h.store.Add(wt)
	if err != nil {
		redirect(w, r, "Saqlab bo'lmadi: "+err.Error(), "err")
		return
	}
	h.watcher.CheckNow(saved.ID)
	redirect(w, r, "Kuzatuv yaratildi, birinchi tekshiruv boshlandi", "ok")
}

func (h *Web) deleteWatch(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Delete(r.PathValue("id")); err != nil {
		redirect(w, r, err.Error(), "err")
		return
	}
	redirect(w, r, "Kuzatuv o'chirildi", "ok")
}

func (h *Web) toggleWatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var nowActive bool
	err := h.store.Update(id, func(x *Watch) {
		x.Active = !x.Active
		nowActive = x.Active
		if x.Active {
			x.LastSig = ""
		}
	})
	if err != nil {
		redirect(w, r, err.Error(), "err")
		return
	}
	if nowActive {
		h.watcher.CheckNow(id)
		redirect(w, r, "Kuzatuv yoqildi", "ok")
		return
	}
	redirect(w, r, "Kuzatuv to'xtatildi", "ok")
}

func (h *Web) checkWatch(w http.ResponseWriter, r *http.Request) {
	h.watcher.CheckNow(r.PathValue("id"))
	redirect(w, r, "Tekshiruv navbatga qo'yildi — bir necha soniyadan keyin sahifani yangilang", "ok")
}

func (h *Web) search(w http.ResponseWriter, r *http.Request) {
	d := h.base("Qidiruv", r)
	q := r.URL.Query()
	d.From, d.To, d.Date = q.Get("from"), q.Get("to"), q.Get("date")
	if d.Date == "" {
		d.Date = d.Today
	}
	if d.From != "" && d.To != "" && q.Get("date") != "" {
		d.Searched = true
		switch {
		case d.Paused:
			d.SearchErr = "Sayt xato qaytargani uchun so'rovlar " + d.PausedUntil + " gacha to'xtatilgan"
		case !h.allowManualSearch():
			d.SearchErr = "Juda tez-tez qidiryapsiz — " + manualSearchGap.String() + " kuting"
		default:
			ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
			defer cancel()
			trains, err := h.rc.Search(ctx, d.From, d.To, d.Date)
			if err != nil {
				d.SearchErr = err.Error()
			} else {
				d.Trains = trains
			}
		}
	}
	h.render(w, "search.html", d)
}

func (h *Web) allowManualSearch() bool {
	h.searchMu.Lock()
	defer h.searchMu.Unlock()
	if time.Since(h.lastSrch) < manualSearchGap {
		return false
	}
	h.lastSrch = time.Now()
	return true
}

func (h *Web) testTelegram(w http.ResponseWriter, r *http.Request) {
	chat := strings.TrimSpace(r.FormValue("chat_id"))
	if chat == "" {
		redirect(w, r, "Chat ID kiriting", "err")
		return
	}
	if err := h.tg.Send(r.Context(), chat, "✅ Test xabar: bilet kuzatuvchi ishlayapti."); err != nil {
		redirect(w, r, err.Error(), "err")
		return
	}
	redirect(w, r, "Test xabar yuborildi", "ok")
}

func (h *Web) lastResponse(w http.ResponseWriter, r *http.Request) {
	raw := h.rc.LastRaw()
	if len(raw) == 0 {
		http.Error(w, "Hali so'rov yuborilmagan", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(raw)
}
