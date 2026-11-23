// Package webui — HTTP orqali UI (delivery qatlami). Faqat usecase interfeyslariga
// bog'liq; HTTP so'rovini usecase chaqiruviga, natijani esa presenter orqali sahifaga aylantiradi.
package webui

import (
	"context"
	"embed"
	"html/template"
	"net/http"
	"net/url"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

//go:embed templates/*.html
var templatesFS embed.FS

// ---- Delivery qatlami kutadigan kirish portlari ----

type AuthUseCase interface {
	LoginURL(state string) string
	Login(ctx context.Context, code string) (token string, user domain.User, expires time.Time, err error)
	Authenticate(ctx context.Context, token string) (domain.User, error)
	Logout(ctx context.Context, token string) error
}

type WatchUseCase interface {
	Create(ctx context.Context, userID int64, in usecase.CreateWatchInput) (domain.Watch, error)
	List(ctx context.Context, userID int64) ([]domain.Watch, error)
	Delete(ctx context.Context, userID, id int64) error
	Toggle(ctx context.Context, userID, id int64) (domain.Watch, error)
	RequestCheck(ctx context.Context, userID, id int64) error
}

type SearchUseCase interface {
	Search(ctx context.Context, userID int64, r domain.Route) ([]domain.Train, error)
}

type TelegramUseCase interface {
	StartLink(ctx context.Context, userID int64) (string, error)
	Unlink(ctx context.Context, userID int64) error
	SendTest(ctx context.Context, userID int64) error
}

type PollingUseCase interface {
	Policy(ctx context.Context) (domain.PollingPolicy, error)
	Update(ctx context.Context, actor domain.User, rules []domain.PollingRule) (domain.PollingPolicy, error)
	Stats(ctx context.Context, actor domain.User) ([]usecase.TierStats, error)
}

type MonitorStatus interface {
	PausedUntil() time.Time
}

type RawResponseSource interface {
	LastRaw() []byte
}

type Deps struct {
	Auth          AuthUseCase
	Watches       WatchUseCase
	Search        SearchUseCase
	Telegram      TelegramUseCase
	Polling       PollingUseCase
	Status        MonitorStatus
	Raw           RawResponseSource
	Stations      usecase.StationDirectory
	Clock         usecase.Clock
	TelegramOK    bool
	MaxWatches    int
	AdminEmails   []string
	SecureCookies bool // BASE_URL https bo'lsa true
}

type Server struct {
	d    Deps
	tmpl *template.Template
	p    presenter
}

func New(d Deps) (*Server, error) {
	t, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{d: d, tmpl: t, p: presenter{stations: d.Stations, clock: d.Clock}}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// ochiq
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("GET /auth/google", s.googleStart)
	mux.HandleFunc("GET /auth/google/callback", s.googleCallback)
	mux.HandleFunc("POST /logout", s.logout)

	// faqat kirganlar uchun
	mux.Handle("GET /{$}", s.auth(s.dashboard))
	mux.Handle("POST /watches", s.auth(s.createWatch))
	mux.Handle("POST /watches/{id}/delete", s.auth(s.deleteWatch))
	mux.Handle("POST /watches/{id}/toggle", s.auth(s.toggleWatch))
	mux.Handle("POST /watches/{id}/check", s.auth(s.checkWatch))
	mux.Handle("GET /search", s.auth(s.search))
	mux.Handle("GET /settings", s.auth(s.settings))
	mux.Handle("POST /settings/telegram/link", s.auth(s.telegramLink))
	mux.Handle("POST /settings/telegram/unlink", s.auth(s.telegramUnlink))
	mux.Handle("POST /settings/telegram/test", s.auth(s.telegramTest))
	mux.Handle("GET /debug/last-response", s.auth(s.lastResponse))
	mux.Handle("GET /admin/polling", s.auth(s.adminPolling))
	mux.Handle("POST /admin/polling", s.auth(s.adminPollingSave))

	return securityHeaders(sameOrigin(mux))
}

// sameOrigin boshqa saytlardan yuborilgan POST so'rovlarni bloklaydi (CSRF).
// Sessiya cookie'si SameSite=Lax bo'lgani bilan birga ikki qavat himoya beradi.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
