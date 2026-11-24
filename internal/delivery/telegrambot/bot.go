// Package telegrambot — Telegram'dan kiruvchi buyruqlar (delivery qatlami).
// Hozircha faqat /start: foydalanuvchiga saytga qanday kirishni aytadi.
// Botga /start yozish, shuningdek, bot foydalanuvchiga xabar yubora olishini ta'minlaydi.
package telegrambot

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/khasansayliev/watcher/internal/adapter/telegram"
)

type Greeter interface {
	Greet(ctx context.Context, chatID string) error
}

type Bot struct {
	client  *telegram.Client
	greeter Greeter
	log     *slog.Logger
}

func New(c *telegram.Client, g Greeter, log *slog.Logger) *Bot {
	return &Bot{client: c, greeter: g, log: log}
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
	cmd, _, _ := strings.Cut(strings.TrimSpace(u.Text), " ")
	if cmd != "/start" {
		return
	}
	err := b.greeter.Greet(ctx, u.ChatID)
	if err != nil {
		b.log.Info("telegram /start", "chat", u.ChatID, "err", err)
	}
}
