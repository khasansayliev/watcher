package main

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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---- Domen modellari (UI va kuzatuvchi shu bilan ishlaydi) ----

type Car struct {
	Type      string  // vagon turi (masalan: Купе, Плацкарт, Сидячий)
	Class     string  // xizmat klassi kodi (masalan: 2E, 1В)
	FreeSeats int     // bo'sh joylar
	MinPrice  float64 // tarif, so'm
}

type Train struct {
	Number    string
	Type      string
	Brand     string
	Departure string
	Arrival   string
	Duration  string
	Cars      []Car
}

// StatusError — API 200 dan boshqa javob qaytarganda.
type StatusError struct {
	Code       int
	RetryAfter time.Duration
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("railway API HTTP %d: %s", e.Code, e.Body)
}

// ---- Klient ----

const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"

type RailwayClient struct {
	base, lang, manualCookie, dataDir string
	hc                                *http.Client

	mu      sync.Mutex
	xsrf    string
	lastRaw []byte
}

func NewRailwayClient(base, manualCookie, lang, dataDir string) *RailwayClient {
	jar, _ := cookiejar.New(nil)
	return &RailwayClient{
		base: base, lang: lang, manualCookie: manualCookie, dataDir: dataDir,
		hc: &http.Client{Timeout: 30 * time.Second, Jar: jar},
	}
}

// token XSRF-TOKEN ni oladi: yo RAILWAY_COOKIE dan, yo bosh sahifani ochib (brauzer kabi).
func (c *RailwayClient) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.xsrf != "" {
		return c.xsrf, nil
	}
	if c.manualCookie != "" {
		for _, part := range strings.Split(c.manualCookie, ";") {
			kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(kv) == 2 && kv[0] == "XSRF-TOKEN" {
				c.xsrf = kv[1]
				return c.xsrf, nil
			}
		}
		return "", errors.New("RAILWAY_COOKIE ichida XSRF-TOKEN topilmadi")
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/"+c.lang+"/", nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("bosh sahifa: %w", err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 2<<20))
	resp.Body.Close()

	u, _ := url.Parse(c.base)
	for _, ck := range c.hc.Jar.Cookies(u) {
		if ck.Name == "XSRF-TOKEN" {
			c.xsrf = ck.Value
			return c.xsrf, nil
		}
	}
	return "", fmt.Errorf("XSRF-TOKEN cookie olinmadi (HTTP %d). Brauzerdan cookie'ni RAILWAY_COOKIE ga bering", resp.StatusCode)
}

func (c *RailwayClient) resetToken() {
	c.mu.Lock()
	c.xsrf = ""
	c.mu.Unlock()
}

// LastRaw — oxirgi API javobi (format moslashtirish uchun).
func (c *RailwayClient) LastRaw() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastRaw
}

// Search tanlangan yo'nalish va sana bo'yicha poyezdlarni qaytaradi. date: YYYY-MM-DD.
func (c *RailwayClient) Search(ctx context.Context, from, to, date string) ([]Train, error) {
	trains, err := c.searchOnce(ctx, from, to, date)
	var se *StatusError
	if errors.As(err, &se) && (se.Code == 401 || se.Code == 403 || se.Code == 419) && c.manualCookie == "" {
		// Token eskirgan bo'lishi mumkin — bir marta yangilab qayta urinamiz.
		c.resetToken()
		trains, err = c.searchOnce(ctx, from, to, date)
	}
	return trains, err
}

func (c *RailwayClient) searchOnce(ctx context.Context, from, to, date string) ([]Train, error) {
	tok, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]any{
		"directions": map[string]any{
			"forward": map[string]string{
				"date":           date,
				"depStationCode": from,
				"arvStationCode": to,
			},
		},
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/v3/handbook/trains/list", bytes.NewReader(body))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", c.lang)
	req.Header.Set("Origin", c.base)
	req.Header.Set("Referer", c.base+"/"+c.lang+"/")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-XSRF-TOKEN", tok)
	if c.manualCookie != "" {
		req.Header.Set("Cookie", c.manualCookie)
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
	c.saveRaw(raw)

	if resp.StatusCode != http.StatusOK {
		se := &StatusError{Code: resp.StatusCode, Body: truncate(string(raw), 300)}
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				se.RetryAfter = time.Duration(secs) * time.Second
			}
		}
		return nil, se
	}
	return parseTrains(raw)
}

func (c *RailwayClient) saveRaw(raw []byte) {
	c.mu.Lock()
	c.lastRaw = raw
	c.mu.Unlock()
	_ = os.WriteFile(filepath.Join(c.dataDir, "last_response.json"), raw, 0o600)
}

// ---- Javobni o'qish ----
//
// DIQQAT: javob formati rasmiy hujjatlashtirilmagan. Quyidagi maydon nomlari
// ochiq manbalardagi kodga asoslangan taxmin. Agar sizdagi javob boshqacha bo'lsa,
// data/last_response.json yoki UI'dagi "Oxirgi javob" sahifasini oching va
// faqat shu bo'limdagi json teglarini moslang.

type apiTariff struct {
	Tariff           flexNum `json:"tariff"`
	ClassServiceType flexStr `json:"classServiceType"`
	FreeSeats        flexNum `json:"freeSeats"`
}

type apiCar struct {
	Type      flexStr     `json:"type"`
	FreeSeats flexNum     `json:"freeSeats"`
	Tariffs   []apiTariff `json:"tariffs"`
}

type apiTrain struct {
	Type          flexStr  `json:"type"`
	Number        flexStr  `json:"number"`
	Brand         flexStr  `json:"brand"`
	DepartureDate flexStr  `json:"departureDate"`
	ArrivalDate   flexStr  `json:"arrivalDate"`
	TimeOnWay     flexStr  `json:"timeOnWay"`
	Cars          []apiCar `json:"cars"`
}

type apiDirections struct {
	Forward struct {
		Trains []apiTrain `json:"trains"`
	} `json:"forward"`
}

type apiResp struct {
	Data *struct {
		Directions *apiDirections `json:"directions"`
	} `json:"data"`
	Directions *apiDirections `json:"directions"`
}

func parseTrains(raw []byte) ([]Train, error) {
	var r apiResp
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("javobni o'qib bo'lmadi: %w", err)
	}
	var d *apiDirections
	switch {
	case r.Data != nil && r.Data.Directions != nil:
		d = r.Data.Directions
	case r.Directions != nil:
		d = r.Directions
	default:
		return nil, errors.New("javobda 'directions' topilmadi — formatni tekshiring (UI → Oxirgi javob)")
	}

	out := make([]Train, 0, len(d.Forward.Trains))
	for _, t := range d.Forward.Trains {
		tr := Train{
			Number:    string(t.Number),
			Type:      string(t.Type),
			Brand:     string(t.Brand),
			Departure: string(t.DepartureDate),
			Arrival:   string(t.ArrivalDate),
			Duration:  string(t.TimeOnWay),
		}
		for _, c := range t.Cars {
			if len(c.Tariffs) == 0 {
				tr.Cars = append(tr.Cars, Car{Type: string(c.Type), FreeSeats: int(c.FreeSeats)})
				continue
			}
			for _, tf := range c.Tariffs {
				seats := int(tf.FreeSeats)
				if seats == 0 {
					seats = int(c.FreeSeats)
				}
				tr.Cars = append(tr.Cars, Car{
					Type:      string(c.Type),
					Class:     string(tf.ClassServiceType),
					FreeSeats: seats,
					MinPrice:  float64(tf.Tariff),
				})
			}
		}
		out = append(out, tr)
	}
	return out, nil
}

// flexStr — satr yoki son kelsa ham qabul qiladi.
type flexStr string

func (f *flexStr) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexStr(s)
		return nil
	}
	*f = flexStr(string(b))
	return nil
}

// flexNum — son yoki "270 000" kabi satr kelsa ham qabul qiladi.
type flexNum float64

func (f *flexNum) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	s = strings.ReplaceAll(strings.ReplaceAll(s, " ", ""), " ", "")
	if s == "" || s == "null" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil // noma'lum format — 0 qoldiramiz, butun javobni buzmaymiz
	}
	*f = flexNum(v)
	return nil
}
