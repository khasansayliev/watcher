// Package webui — HTTP orqali UI (delivery qatlami). Faqat usecase interfeyslariga
// bog'liq; HTTP so'rovini usecase chaqiruviga, natijani esa presenter orqali sahifaga aylantiradi.
package webui

import (
	"context"
	"crypto/subtle"
	"embed"
	"html/template"
	"net/http"
	"net/url"
	"time"

	"github.com/khasansayliev/railbot/internal/domain"
	"github.com/khasansayliev/railbot/internal/usecase"
)

//go:embed templates/*.html
var templatesFS embed.FS

// Delivery qatlami kutadigan kirish portlari.

type WatchUseCase interface {
	Create(ctx context.Context, in usecase.CreateWatchInput) (domain.Watch, error)
	List(ctx context.Context) ([]domain.Watch, error)
	Delete(ctx context.Context, id string) error
	Toggle(ctx context.Context, id string) (domain.Watch, error)
	RequestCheck(ctx context.Context, id string) error
	SendTest(ctx context.Context, chatID string) error
}

type SearchUseCase interface {
	Search(ctx context.Context, r domain.Route) ([]domain.Train, error)
}

type MonitorStatus interface {
	Interval() time.Duration
	PausedUntil() time.Time
}

type RawResponseSource interface {
	LastRaw() []byte
}

type Deps struct {
	Watches       WatchUseCase
	Search        SearchUseCase
	Status        MonitorStatus
	Raw           RawResponseSource
	Stations      usecase.StationDirectory
	Clock         usecase.Clock
	TelegramOK    bool
	DefaultChatID string
	AuthUser      string
	AuthPassword  string // bo'sh bo'lsa auth o'chiq
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
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("POST /watches", s.createWatch)
	mux.HandleFunc("POST /watches/{id}/delete", s.deleteWatch)
	mux.HandleFunc("POST /watches/{id}/toggle", s.toggleWatch)
	mux.HandleFunc("POST /watches/{id}/check", s.checkWatch)
	mux.HandleFunc("GET /search", s.search)
	mux.HandleFunc("POST /telegram/test", s.testTelegram)
	mux.HandleFunc("GET /debug/last-response", s.lastResponse)
	return s.basicAuth(sameOrigin(mux))
}

func (s *Server) basicAuth(next http.Handler) http.Handler {
	if s.d.AuthPassword == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(s.d.AuthUser)) != 1 ||
			subtle.ConstantTimeCompare([]byte(p), []byte(s.d.AuthPassword)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="railbot"`)
			http.Error(w, "Kirish taqiqlangan", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sameOrigin boshqa saytlardan yuborilgan POST so'rovlarni bloklaydi.
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
