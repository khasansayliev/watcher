package webui

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

const (
	sessionCookie = "watcher_session"
	loginCookie   = "watcher_login" // brauzerga bog'langan kirish kodi (HttpOnly)
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

// loginPage "Telegram orqali kirish" tugmasini ko'rsatadi.
func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if _, err := s.currentUser(r); err == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	vm := loginVM{Error: r.URL.Query().Get("err")}
	if link, err := s.d.Auth.BotURL("login"); err == nil { // t.me/<bot>?start=login
		vm.CodeBotURL = link
	}
	s.render(w, "login.html", vm)
}

// loginStart yangi kirish so'rovi ochadi. Kod faqat shu brauzerning HttpOnly cookie'sida turadi:
// havolani boshqa odam ochsa ham, sessiyani faqat kodni ochgan brauzer oladi.
func (s *Server) loginStart(w http.ResponseWriter, r *http.Request) {
	code, _, err := s.d.Auth.BeginLogin(r.Context())
	if err != nil {
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			loginError(w, r, ve.Msg)
			return
		}
		slog.Error("kirish so'rovi", "err", err)
		loginError(w, r, "Kirishni boshlab bo'lmadi, keyinroq urinib ko'ring")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: loginCookie, Value: code, Path: "/login",
		MaxAge: 600, HttpOnly: true, Secure: s.d.SecureCookies, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login/wait", http.StatusSeeOther)
}

// codeLogin — Telegram o'rnatilmagan qurilmada: botdan (boshqa qurilmada) olingan bir martalik kod bilan kirish.
func (s *Server) codeLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	if err := r.ParseForm(); err != nil {
		loginError(w, r, "Noto'g'ri so'rov")
		return
	}
	token, _, expires, err := s.d.Auth.LoginWithCode(r.Context(), r.PostFormValue("code"), clientKey(r))
	if err != nil {
		var ve *domain.ValidationError
		switch {
		case errors.As(err, &ve):
			loginError(w, r, ve.Msg)
		case errors.Is(err, usecase.ErrTooManyAttempts):
			slog.Warn("kod bilan kirish: urinishlar chegarasi", "client", clientKey(r))
			loginError(w, r, "Juda ko'p noto'g'ri urinish. Bir necha daqiqadan keyin qayta urinib ko'ring")
		case errors.Is(err, domain.ErrForbidden):
			loginError(w, r, "Akkauntingiz administrator tomonidan o'chirilgan")
		default:
			slog.Error("kod bilan kirish", "err", err)
			loginError(w, r, "Kod bilan kirib bo'lmadi")
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

// loginWait kutish sahifasi: botda tasdiqlangunga qadar o'zini yangilab turadi,
// tasdiqlangach sessiya cookie'sini beradi va bosh sahifaga o'tkazadi.
func (s *Server) loginWait(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(loginCookie)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	token, _, expires, err := s.d.Auth.PollLogin(r.Context(), c.Value)
	switch {
	case err == nil:
		clearLoginCookie(w, s.d.SecureCookies)
		http.SetCookie(w, &http.Cookie{
			Name: sessionCookie, Value: token, Path: "/",
			Expires: expires, MaxAge: int(time.Until(expires).Seconds()),
			HttpOnly: true, Secure: s.d.SecureCookies, SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	case errors.Is(err, usecase.ErrLoginPending):
		link, lerr := s.d.Auth.BotURL(c.Value)
		if lerr != nil {
			slog.Error("bot havolasi", "err", lerr)
			loginError(w, r, "Telegram bot bilan bog'lanib bo'lmadi")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		s.render(w, "login_wait.html", loginWaitVM{BotURL: link})
	case errors.Is(err, domain.ErrForbidden):
		clearLoginCookie(w, s.d.SecureCookies)
		loginError(w, r, "Akkauntingiz administrator tomonidan o'chirilgan")
	case errors.Is(err, domain.ErrNotFound):
		clearLoginCookie(w, s.d.SecureCookies)
		loginError(w, r, "Kirish havolasi eskirdi, qaytadan urinib ko'ring")
	default:
		slog.Error("kirishni tekshirish", "err", err)
		loginError(w, r, "Ichki xato yuz berdi")
	}
}

func clearLoginCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: loginCookie, Value: "", Path: "/login", MaxAge: -1,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
}

// clientKey — urinishlarni cheklash uchun so'rov egasi. So'rov bevosita internetdan kelsa, bu uning manzili.
// So'rov shu mashinadagi/Docker tarmog'idagi reverse proxy'dan (loopback yoki shaxsiy manzil) kelsa,
// haqiqiy manzil X-Forwarded-For ning oxirgi yozuvida (proxy o'zi qo'shgan).
// Internetdan kelgan so'rovda sarlavha e'tiborga olinmaydi, shuning uchun uni soxtalashtirib cheklovni aylanib o'tib bo'lmaydi.
func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if last := strings.TrimSpace(parts[len(parts)-1]); net.ParseIP(last) != nil {
				return last
			}
		}
	}
	return host
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
