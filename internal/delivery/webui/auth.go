package webui

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

const (
	sessionCookie = "watcher_session"
	// loginCookie login sahifasi ochilganda qo'yiladi va callback'da talab qilinadi:
	// boshqa birov tayyorlab yuborgan (o'zining imzolangan) havola bilan
	// foydalanuvchini begona akkauntga kiritib qo'yishning (login CSRF) oldini oladi.
	loginCookie   = "watcher_login"
	callbackPath  = "/auth/telegram/callback"
	loginStateAge = 10 * time.Minute
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

// loginPage Telegram Login Widget'ni ko'rsatadi.
func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if _, err := s.currentUser(r); err == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	nonce, err := usecase.RandomToken(16)
	if err != nil {
		http.Error(w, "server xatosi", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: loginCookie, Value: nonce, Path: callbackPath,
		MaxAge: int(loginStateAge.Seconds()), HttpOnly: true, Secure: s.d.SecureCookies, SameSite: http.SameSiteLaxMode,
	})
	s.render(w, "login.html", loginVM{
		Error:       r.URL.Query().Get("err"),
		BotUsername: s.d.BotUsername,
		AuthURL:     s.d.BaseURL + callbackPath,
	})
}

// telegramCallback — widget foydalanuvchi tasdiqlagach shu manzilga imzolangan maydonlar bilan yo'naltiradi:
// ?id=...&first_name=...&username=...&photo_url=...&auth_date=...&hash=...
func (s *Server) telegramCallback(w http.ResponseWriter, r *http.Request) {
	if _, err := r.Cookie(loginCookie); err != nil {
		loginError(w, r, "Kirish sessiyasi topilmadi — qaytadan urinib ko'ring")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: loginCookie, Value: "", Path: callbackPath, MaxAge: -1})

	fields := map[string]string{}
	for k, v := range r.URL.Query() {
		if len(v) != 1 {
			loginError(w, r, "Noto'g'ri so'rov")
			return
		}
		fields[k] = v[0]
	}

	token, _, expires, err := s.d.Auth.Login(r.Context(), fields)
	if err != nil {
		var ve *domain.ValidationError
		switch {
		case errors.As(err, &ve):
			loginError(w, r, ve.Msg)
		case errors.Is(err, usecase.ErrBadSignature):
			slog.Warn("telegram login: imzo noto'g'ri", "remote", r.RemoteAddr)
			loginError(w, r, "Telegram ma'lumoti tasdiqlanmadi")
		default:
			slog.Error("telegram login", "err", err)
			loginError(w, r, "Telegram orqali kirib bo'lmadi")
		}
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
