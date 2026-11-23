// Package google — Google OAuth 2.0 / OpenID Connect adapteri (usecase.IdentityProvider).
// Tashqi kutubxonasiz: authorization code oqimi standart net/http bilan.
package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

const (
	defaultAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	defaultTokenURL    = "https://oauth2.googleapis.com/token"
	defaultUserinfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
)

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string // https://sizning-domen/auth/google/callback

	// Testlar uchun almashtiriladi; bo'sh bo'lsa Google manzillari.
	AuthURL, TokenURL, UserinfoURL string
}

type Provider struct {
	cfg Config
	hc  *http.Client
}

var _ usecase.IdentityProvider = (*Provider)(nil)

func New(cfg Config) *Provider {
	if cfg.AuthURL == "" {
		cfg.AuthURL = defaultAuthURL
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = defaultTokenURL
	}
	if cfg.UserinfoURL == "" {
		cfg.UserinfoURL = defaultUserinfoURL
	}
	return &Provider{cfg: cfg, hc: &http.Client{Timeout: 15 * time.Second}}
}

func (p *Provider) AuthURL(state string) string {
	q := url.Values{
		"client_id":     {p.cfg.ClientID},
		"redirect_uri":  {p.cfg.RedirectURL},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"state":         {state},
		"prompt":        {"select_account"},
	}
	return p.cfg.AuthURL + "?" + q.Encode()
}

// Exchange kodni access token'ga almashtiradi va foydalanuvchi ma'lumotini oladi.
// Ma'lumot to'g'ridan-to'g'ri Google'dan TLS orqali olingani uchun id_token imzosini alohida tekshirish shart emas.
func (p *Provider) Exchange(ctx context.Context, code string) (domain.Identity, error) {
	if code == "" {
		return domain.Identity{}, errors.New("kod bo'sh")
	}
	form := url.Values{
		"code":          {code},
		"client_id":     {p.cfg.ClientID},
		"client_secret": {p.cfg.ClientSecret},
		"redirect_uri":  {p.cfg.RedirectURL},
		"grant_type":    {"authorization_code"},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.TokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := p.doJSON(req, &tok); err != nil {
		return domain.Identity{}, fmt.Errorf("token: %w", err)
	}
	if tok.AccessToken == "" {
		return domain.Identity{}, fmt.Errorf("token: %s %s", tok.Error, tok.Description)
	}

	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.UserinfoURL, nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	var ui struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := p.doJSON(req, &ui); err != nil {
		return domain.Identity{}, fmt.Errorf("userinfo: %w", err)
	}
	return domain.Identity{Subject: ui.Sub, Email: ui.Email, EmailVerified: ui.EmailVerified, Name: ui.Name, Picture: ui.Picture}, nil
}

func (p *Provider) doJSON(req *http.Request, out any) error {
	resp, err := p.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("HTTP %d: javobni o'qib bo'lmadi", resp.StatusCode)
	}
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}
