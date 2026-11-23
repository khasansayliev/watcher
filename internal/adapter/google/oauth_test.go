package google

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAuthURLAndExchange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_ = r.ParseForm()
			if r.Form.Get("code") != "abc" || r.Form.Get("client_secret") != "sec" || r.Form.Get("grant_type") != "authorization_code" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "at-1"})
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer at-1" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{}`))
				return
			}
			_, _ = w.Write([]byte(`{"sub":"123","email":"ali@example.com","email_verified":true,"name":"Ali","picture":"https://p"}`))
		}
	}))
	defer srv.Close()

	p := New(Config{ClientID: "cid", ClientSecret: "sec", RedirectURL: "https://app/cb",
		AuthURL: srv.URL + "/auth", TokenURL: srv.URL + "/token", UserinfoURL: srv.URL + "/userinfo"})

	u, _ := url.Parse(p.AuthURL("st8"))
	q := u.Query()
	if q.Get("state") != "st8" || q.Get("redirect_uri") != "https://app/cb" || !strings.Contains(q.Get("scope"), "email") {
		t.Fatalf("auth url: %s", u)
	}

	id, err := p.Exchange(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject != "123" || id.Email != "ali@example.com" || !id.EmailVerified {
		t.Fatalf("identity: %+v", id)
	}

	if _, err := p.Exchange(context.Background(), "wrong"); err == nil {
		t.Fatal("noto'g'ri kod rad etilishi kerak")
	}
}
