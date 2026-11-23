// Package railway — eticket.railway.uz ichki API'si uchun adapter (usecase.TrainSearcher).
package railway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/khasansayliev/railbot/internal/domain"
	"github.com/khasansayliev/railbot/internal/pkg/textfmt"
	"github.com/khasansayliev/railbot/internal/usecase"
)

const (
	searchPath = "/api/v3/handbook/trains/list"
	userAgent  = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"
)

type Config struct {
	BaseURL      string // https://eticket.railway.uz
	Lang         string // uz
	ManualCookie string // ixtiyoriy: brauzerdan olingan cookie qatori
	RawDumpPath  string // ixtiyoriy: oxirgi javobni shu faylga yozish
}

type Client struct {
	cfg Config
	hc  *http.Client

	mu      sync.Mutex
	xsrf    string
	lastRaw []byte
}

var _ usecase.TrainSearcher = (*Client)(nil)

func New(cfg Config) *Client {
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	jar, _ := cookiejar.New(nil)
	return &Client{cfg: cfg, hc: &http.Client{Timeout: 30 * time.Second, Jar: jar}}
}

// BuyURL — foydalanuvchini yo'naltiriladigan rasmiy sahifa.
func (c *Client) BuyURL() string { return c.cfg.BaseURL + "/" + c.cfg.Lang + "/" }

// LastRaw — oxirgi API javobi (format moslashtirish uchun).
func (c *Client) LastRaw() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastRaw
}

func (c *Client) Search(ctx context.Context, r domain.Route) ([]domain.Train, error) {
	trains, err := c.searchOnce(ctx, r)
	var ue *usecase.UpstreamError
	if errors.As(err, &ue) && (ue.Status == 401 || ue.Status == 403 || ue.Status == 419) && c.cfg.ManualCookie == "" {
		c.resetToken() // token eskirgan bo'lishi mumkin — bir marta qayta urinamiz
		trains, err = c.searchOnce(ctx, r)
	}
	return trains, err
}

func (c *Client) searchOnce(ctx context.Context, r domain.Route) ([]domain.Train, error) {
	tok, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(searchRequest{Directions: directionsReq{Forward: forwardReq{
		Date: r.Date, DepStationCode: r.From, ArvStationCode: r.To,
	}}})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+searchPath, bytes.NewReader(body))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", c.cfg.Lang)
	req.Header.Set("Origin", c.cfg.BaseURL)
	req.Header.Set("Referer", c.BuyURL())
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-XSRF-TOKEN", tok)
	if c.cfg.ManualCookie != "" {
		req.Header.Set("Cookie", c.cfg.ManualCookie)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, err
	}
	c.keepRaw(raw)

	if resp.StatusCode != http.StatusOK {
		ue := &usecase.UpstreamError{Status: resp.StatusCode, Msg: textfmt.Truncate(string(raw), 300)}
		if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
			ue.RetryAfter = time.Duration(secs) * time.Second
		}
		return nil, ue
	}
	return decodeTrains(raw)
}

// token XSRF-TOKEN ni oladi: RAILWAY_COOKIE dan yoki bosh sahifani ochib (brauzer kabi).
func (c *Client) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.xsrf != "" {
		return c.xsrf, nil
	}
	if c.cfg.ManualCookie != "" {
		for _, part := range strings.Split(c.cfg.ManualCookie, ";") {
			if k, v, ok := strings.Cut(strings.TrimSpace(part), "="); ok && k == "XSRF-TOKEN" {
				c.xsrf = v
				return v, nil
			}
		}
		return "", errors.New("RAILWAY_COOKIE ichida XSRF-TOKEN topilmadi")
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.BuyURL(), nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("bosh sahifa: %w", err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 2<<20))
	resp.Body.Close()

	u, _ := url.Parse(c.cfg.BaseURL)
	for _, ck := range c.hc.Jar.Cookies(u) {
		if ck.Name == "XSRF-TOKEN" {
			c.xsrf = ck.Value
			return ck.Value, nil
		}
	}
	return "", fmt.Errorf("XSRF-TOKEN cookie olinmadi (HTTP %d). Brauzerdan cookie'ni RAILWAY_COOKIE ga bering", resp.StatusCode)
}

func (c *Client) resetToken() {
	c.mu.Lock()
	c.xsrf = ""
	c.mu.Unlock()
}

func (c *Client) keepRaw(raw []byte) {
	c.mu.Lock()
	c.lastRaw = raw
	c.mu.Unlock()
	if c.cfg.RawDumpPath != "" {
		_ = os.WriteFile(c.cfg.RawDumpPath, raw, 0o600)
	}
}
