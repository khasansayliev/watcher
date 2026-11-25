package webui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

type fakeAuth struct {
	AuthUseCase
	gotCode, gotKey string
	err             error
}

func (f *fakeAuth) LoginWithCode(_ context.Context, code, key string) (string, domain.User, time.Time, error) {
	f.gotCode, f.gotKey = code, key
	if f.err != nil {
		return "", domain.User{}, time.Time{}, f.err
	}
	return "tok123", domain.User{ID: 1}, time.Now().Add(time.Hour), nil
}
func (f *fakeAuth) Authenticate(context.Context, string) (domain.User, error) {
	return domain.User{}, domain.ErrUnauthorized
}

func newTestServer(t *testing.T, a AuthUseCase) http.Handler {
	t.Helper()
	s, err := New(Deps{Auth: a, BotUsername: "watcher_bot", BaseURL: "https://watcher.test", SecureCookies: true})
	if err != nil {
		t.Fatal(err)
	}
	return s.Handler()
}

func postCode(h http.Handler, code string, mod func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/login/code", strings.NewReader(url.Values{"code": {code}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "watcher.test"
	req.RemoteAddr = "203.0.113.9:5555"
	if mod != nil {
		mod(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLoginPageShowsCodeForm(t *testing.T) {
	h := newTestServer(t, &fakeAuth{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
	body := rec.Body.String()
	for _, want := range []string{`action="/login/code"`, `name="code"`, "https://t.me/watcher_bot?start=login", "/login"} {
		if !strings.Contains(body, want) {
			t.Errorf("login sahifasida %q yo'q", want)
		}
	}
}

func TestCodeLoginSuccessSetsSessionCookie(t *testing.T) {
	fa := &fakeAuth{}
	rec := postCode(newTestServer(t, fa), "abcd-efgh", nil)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("yo'naltirish: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	var c *http.Cookie
	for _, x := range rec.Result().Cookies() {
		if x.Name == sessionCookie {
			c = x
		}
	}
	if c == nil || c.Value != "tok123" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("sessiya cookie'si xavfsiz bo'lishi kerak: %+v", c)
	}
	if fa.gotCode != "abcd-efgh" || fa.gotKey != "203.0.113.9" {
		t.Fatalf("usecase'ga uzatilgan: %q %q", fa.gotCode, fa.gotKey)
	}
}

func TestCodeLoginErrors(t *testing.T) {
	cases := map[string]struct {
		err  error
		want string
	}{
		"noto'g'ri kod": {domain.NewValidationError("Kod noto'g'ri"), "Kod+noto%27g%27ri"},
		"chegara":       {usecase.ErrTooManyAttempts, "Juda+ko%27p"},
		"ichki xato":    {errors.New("db qulagan"), "Kod+bilan+kirib"},
	}
	for name, c := range cases {
		rec := postCode(newTestServer(t, &fakeAuth{err: c.err}), "ABCD-EFGH", nil)
		loc := rec.Header().Get("Location")
		if rec.Code != http.StatusSeeOther || !strings.HasPrefix(loc, "/login?err=") || !strings.Contains(loc, c.want) {
			t.Errorf("%s: %d %q", name, rec.Code, loc)
		}
		if len(rec.Result().Cookies()) != 0 {
			t.Errorf("%s: xatoda cookie qo'yilmasligi kerak", name)
		}
		if strings.Contains(loc, "db") {
			t.Errorf("%s: ichki xato matni foydalanuvchiga chiqmasligi kerak: %q", name, loc)
		}
	}
}

func TestCodeLoginRejectsCrossOrigin(t *testing.T) {
	rec := postCode(newTestServer(t, &fakeAuth{}), "ABCD-EFGH", func(r *http.Request) {
		r.Header.Set("Origin", "https://evil.example")
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("boshqa saytdan yuborilgan forma rad etilishi kerak (login CSRF): %d", rec.Code)
	}
}

func TestClientKey(t *testing.T) {
	mk := func(remote, xff string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.RemoteAddr = remote
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return r
	}
	cases := []struct {
		name string
		r    *http.Request
		want string
	}{
		{"to'g'ridan-to'g'ri", mk("203.0.113.9:1", ""), "203.0.113.9"},
		{"internetdan kelgan XFF soxta — e'tiborsiz", mk("203.0.113.9:1", "1.1.1.1"), "203.0.113.9"},
		{"loopback proxy", mk("127.0.0.1:1", "198.51.100.7"), "198.51.100.7"},
		{"docker proxy", mk("172.18.0.1:1", "198.51.100.7"), "198.51.100.7"},
		{"proxy qo'shgan oxirgi yozuv, soxta oldingisi emas", mk("127.0.0.1:1", "6.6.6.6, 198.51.100.7"), "198.51.100.7"},
		{"yaroqsiz XFF", mk("127.0.0.1:1", "garbage"), "127.0.0.1"},
		{"IPv6", mk("[2001:db8::1]:1", ""), "2001:db8::1"},
	}
	for _, c := range cases {
		if got := clientKey(c.r); got != c.want {
			t.Errorf("%s: %q, kutilgan %q", c.name, got, c.want)
		}
	}
}
