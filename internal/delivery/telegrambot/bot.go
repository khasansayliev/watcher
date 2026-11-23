// Package telegrambot — Telegram'dan kiruvchi buyruqlar (delivery qatlami).
// Bot "/start <kod>" ni oladi va usecase'ga uzatadi; o'zi biznes mantiq saqlamaydi.
package telegrambot

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/khasansayliev/railbot/internal/adapter/telegram"
)

type Linker interface {
	LinkChat(ctx context.Context, code, chatID string) error
	Greet(ctx context.Context, chatID string) error
}

type Bot struct {
	client *telegram.Client
	linker Linker
	log    *slog.Logger
}

func New(c *telegram.Client, l Linker, log *slog.Logger) *Bot {
	return &Bot{client: c, linker: l, log: log}
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
	if u.ChatID == "" {
		return
	}
	cmd, arg, _ := strings.Cut(strings.TrimSpace(u.Text), " ")
	if cmd != "/start" {
		return
	}
	var err error
	if arg = strings.TrimSpace(arg); arg == "" {
		err = b.linker.Greet(ctx, u.ChatID)
	} else {
		err = b.linker.LinkChat(ctx, arg, u.ChatID)
	}
	if err != nil {
		b.log.Info("telegram /start", "chat", u.ChatID, "err", err)
	}
}
