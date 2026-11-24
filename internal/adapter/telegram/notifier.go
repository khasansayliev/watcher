package telegram

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strings"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/pkg/textfmt"
	"github.com/khasansayliev/watcher/internal/usecase"
)

const maxMessageLen = 3500 // Telegram limiti 4096

// Notifier — usecase.Notifier ning Telegram amalga oshirilishi.
type Notifier struct {
	c        *Client
	buyURL   string
	stations usecase.StationDirectory
}

var _ usecase.Notifier = (*Notifier)(nil)

func NewNotifier(c *Client, buyURL string, stations usecase.StationDirectory) *Notifier {
	return &Notifier{c: c, buyURL: buyURL, stations: stations}
}

func (n *Notifier) Enabled() bool { return n.c.Enabled() }

func (n *Notifier) SendText(ctx context.Context, chatID, text string) error {
	return n.c.SendMessage(ctx, chatID, text, false)
}

func (n *Notifier) BotURL(code string) (string, error) {
	name, err := n.c.Username(context.Background())
	if err != nil {
		return "", err
	}
	return "https://t.me/" + name + "?start=" + url.QueryEscape(code), nil
}

func (n *Notifier) NotifyAvailability(ctx context.Context, chatID string, r domain.Route, ms []domain.Match) error {
	return n.c.SendMessage(ctx, chatID, n.format(r, ms), true)
}

func (n *Notifier) format(r domain.Route, ms []domain.Match) string {
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
