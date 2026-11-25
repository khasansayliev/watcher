// Package telegrambot — Telegram'dan kiruvchi buyruqlar (delivery qatlami).
// /start — saytga qanday kirishni aytadi (botga /start yozish bot foydalanuvchiga xabar yubora olishini ham ta'minlaydi).
// /login (yoki /start login) — Telegram'i yo'q qurilmadan saytga kirish uchun bir martalik kod beradi.
package telegrambot

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/khasansayliev/watcher/internal/adapter/telegram"
	"github.com/khasansayliev/watcher/internal/domain"
)

type Service interface {
	Greet(ctx context.Context, chatID string) error
	SendLoginCode(ctx context.Context, ident domain.Identity) error
}

type Bot struct {
	client *telegram.Client
	svc    Service
	log    *slog.Logger
}

func New(c *telegram.Client, svc Service, log *slog.Logger) *Bot {
	return &Bot{client: c, svc: svc, log: log}
}

// Run long polling sikli. Bot uchun webhook o'rnatilmagan bo'lishi kerak.
func (b *Bot) Run(ctx context.Context) {
	if !b.client.Enabled() {
		return
	}
	var offset int64
	backoff := time.Second
	for ctx.Err() == nil {
		ups, err := b.client.GetUpdates(ctx, offset, 30*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			b.log.Warn("telegram getUpdates", "err", err, "retry", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, time.Minute)
			continue
		}
		backoff = time.Second
		for _, u := range ups {
			offset = u.ID + 1
			b.handle(ctx, u)
		}
	}
}

func (b *Bot) handle(ctx context.Context, u telegram.Update) {
	if u.ChatID == "" || u.FromID == 0 {
		return
	}
	cmd, arg := parseCommand(u.Text)
	switch {
	case cmd == "/login", cmd == "/start" && arg == "login": // t.me/<bot>?start=login — saytdagi tugma
		ident := domain.Identity{TelegramID: u.FromID, FirstName: u.FirstName, LastName: u.LastName, Username: u.Username}
		if err := b.svc.SendLoginCode(ctx, ident); err != nil {
			b.log.Warn("telegram /login", "chat", u.ChatID, "err", err)
		}
	case cmd == "/start":
		if err := b.svc.Greet(ctx, u.ChatID); err != nil {
			b.log.Info("telegram /start", "chat", u.ChatID, "err", err)
		}
	}
}

// parseCommand "/login@bot arg" ni ("/login", "arg") ga ajratadi.
func parseCommand(text string) (cmd, arg string) {
	cmd, arg, _ = strings.Cut(strings.TrimSpace(text), " ")
	cmd, _, _ = strings.Cut(cmd, "@")
	return strings.ToLower(cmd), strings.TrimSpace(arg)
}
