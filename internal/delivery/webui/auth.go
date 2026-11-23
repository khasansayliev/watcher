package webui

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

const (
	sessionCookie = "watcher_session"
	stateCookie   = "watcher_oauth_state"
)

type ctxKey struct{}

func userFrom(ctx context.Context) domain.User {
	u, _ := ctx.Value(ctxKey{}).(domain.User)
	return u
}

// auth sessiyani tekshiradi; kirmaganlarni /login ga yuboradi.
func (s *Server) auth(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := s.currentUser(r)
		if err != nil {
			if !errors.Is(err, domain.ErrUnauthorized) {
				slog.Error("sessiya tekshiruvi", "err", err)
			}
			if r.Method == http.MethodGet {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			http.Error(w, "Tizimga kiring", http.StatusUnauthorized)
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

func (s *Server) currentUser(r *http.Request) (domain.User, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return domain.User{}, domain.ErrUnauthorized
	}
	return s.d.Auth.Authenticate(r.Context(), c.Value)
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if _, err := s.currentUser(r); err == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	vm := loginVM{Error: r.URL.Query().Get("err")}
	s.render(w, "login.html", vm)
}

// googleStart CSRF uchun tasodifiy state yaratib, Google'ga yo'naltiradi.
func (s *Server) googleStart(w http.ResponseWriter, r *http.Request) {
	state, err := usecase.RandomToken(24)
	if err != nil {
		http.Error(w, "server xatosi", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: state, Path: "/auth/google",
		MaxAge: 600, HttpOnly: true, Secure: s.d.SecureCookies, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, s.d.Auth.LoginURL(state), http.StatusFound)
}

func (s *Server) googleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c, err := r.Cookie(stateCookie)
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: "", Path: "/auth/google", MaxAge: -1})
	if e := q.Get("error"); e != "" {
		loginError(w, r, "Google kirishni bekor qildi")
		return
	}
	if err != nil || q.Get("state") == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(q.Get("state"))) != 1 {
		loginError(w, r, "Sessiya muddati o'tdi, qaytadan urinib ko'ring")
		return
	}

	token, _, expires, err := s.d.Auth.Login(r.Context(), q.Get("code"))
	if err != nil {
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			loginError(w, r, ve.Msg)
			return
		}
		slog.Error("google login", "err", err)
		loginError(w, r, "Google orqali kirib bo'lmadi")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/",
		Expires: expires, MaxAge: int(time.Until(expires).Seconds()),
		HttpOnly: true, Secure: s.d.SecureCookies, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = s.d.Auth.Logout(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.d.SecureCookies, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func loginError(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, "/login?err="+urlEscape(msg), http.StatusSeeOther)
}
