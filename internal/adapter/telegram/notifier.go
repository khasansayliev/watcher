// Package telegram — Telegram Bot API orqali xabar yuboruvchi adapter (usecase.Notifier).
package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/khasansayliev/railbot/internal/domain"
	"github.com/khasansayliev/railbot/internal/pkg/textfmt"
	"github.com/khasansayliev/railbot/internal/usecase"
)

const maxMessageLen = 3500 // Telegram limiti 4096

type Notifier struct {
	token    string
	buyURL   string
	stations usecase.StationDirectory
	hc       *http.Client
}

var _ usecase.Notifier = (*Notifier)(nil)

func New(token, buyURL string, stations usecase.StationDirectory) *Notifier {
	return &Notifier{token: token, buyURL: buyURL, stations: stations, hc: &http.Client{Timeout: 15 * time.Second}}
}

func (n *Notifier) Enabled() bool { return n.token != "" }

func (n *Notifier) NotifyAvailability(ctx context.Context, chatID string, r domain.Route, ms []domain.Match) error {
	return n.send(ctx, chatID, n.formatAvailability(r, ms))
}

func (n *Notifier) SendTest(ctx context.Context, chatID string) error {
	return n.send(ctx, chatID, "✅ Test xabar: bilet kuzatuvchi ishlayapti.")
}

func (n *Notifier) formatAvailability(r domain.Route, ms []domain.Match) string {
	e := html.EscapeString
	var b strings.Builder
	fmt.Fprintf(&b, "🚆 <b>Joy bor!</b> %s → %s, %s\n", e(n.stations.Name(r.From)), e(n.stations.Name(r.To)), r.Date)
	for _, m := range ms {
		t := m.Train
		fmt.Fprintf(&b, "\n<b>%s %s</b>  %s → %s", e(t.Number), e(t.Label()), e(t.Departure), e(t.Arrival))
		if t.Duration != "" {
			fmt.Fprintf(&b, " (%s)", e(t.Duration))
		}
		b.WriteString("\n")
		for _, c := range m.Cars {
			fmt.Fprintf(&b, "  • %s: %d joy", e(c.Label()), c.FreeSeats)
			if c.Price > 0 {
				fmt.Fprintf(&b, ", %s so'm", textfmt.Sum(c.Price))
			}
			b.WriteString("\n")
		}
		if b.Len() > maxMessageLen {
			b.WriteString("\n…")
			break
		}
	}
	fmt.Fprintf(&b, "\nSotib olish: %s", e(n.buyURL))
	return b.String()
}

func (n *Notifier) send(ctx context.Context, chatID, text string) error {
	if n.token == "" {
		return errors.New("TELEGRAM_BOT_TOKEN berilmagan")
	}
	form := url.Values{
		"chat_id":                  {chatID},
		"text":                     {text},
		"parse_mode":               {"HTML"},
		"disable_web_page_preview": {"true"},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.telegram.org/bot"+n.token+"/sendMessage", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := n.hc.Do(req)
	if err != nil {
		// xato matnida URL (demak token) bo'lishi mumkin — yashiramiz
		return fmt.Errorf("telegram: %s", strings.ReplaceAll(err.Error(), n.token, "***"))
	}
	defer resp.Body.Close()

	var r struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("telegram: HTTP %d", resp.StatusCode)
	}
	if !r.OK {
		return fmt.Errorf("telegram: %s", r.Description)
	}
	return nil
}
