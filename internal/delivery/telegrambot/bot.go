// Package telegrambot — Telegram'dan kiruvchi buyruqlar (delivery qatlami).
// Bot "/start <kod>" (saytga kirish), "/login" (Telegram'siz qurilma uchun kod) va "/logout" ni oladi va usecase'ga uzatadi;
// o'zi biznes mantiq saqlamaydi.
package telegrambot

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/khasansayliev/watcher/internal/adapter/telegram"
	"github.com/khasansayliev/watcher/internal/domain"
)

// LoginHandler — botdan kelgan kirish buyruqlarini qayta ishlaydigan usecase.
type LoginHandler interface {
	ConfirmLogin(ctx context.Context, code string, p domain.TelegramProfile) error
	Greet(ctx context.Context, chatID string) error
	// SendLoginCode — Telegram'i yo'q qurilmadan kirish uchun bir martalik kodni shu chatga yuboradi.
	SendLoginCode(ctx context.Context, p domain.TelegramProfile) error
	LogoutAll(ctx context.Context, telegramID string) error
}

type Bot struct {
	client *telegram.Client
	login  LoginHandler
	log    *slog.Logger
}

func New(c *telegram.Client, l LoginHandler, log *slog.Logger) *Bot {
	return &Bot{client: c, login: l, log: log}
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
	if at := strings.IndexByte(cmd, '@'); at >= 0 { // "/start@my_bot"
		cmd = cmd[:at]
	}
	var err error
	switch cmd {
	case "/login": // Telegram'i yo'q qurilma uchun kod
		err = b.login.SendLoginCode(ctx, domain.TelegramProfile{ID: u.ChatID, Username: u.Username, Name: u.Name})
	case "/start":
		switch arg = strings.TrimSpace(arg); arg {
		case "":
			err = b.login.Greet(ctx, u.ChatID)
		case "login": // t.me/<bot>?start=login — saytdagi "kod olish" havolasi
			err = b.login.SendLoginCode(ctx, domain.TelegramProfile{ID: u.ChatID, Username: u.Username, Name: u.Name})
		default:
			err = b.login.ConfirmLogin(ctx, arg, domain.TelegramProfile{ID: u.ChatID, Username: u.Username, Name: u.Name})
		}
	case "/logout":
		if err = b.login.LogoutAll(ctx, u.ChatID); err == nil {
			err = b.client.SendMessage(ctx, u.ChatID, "Barcha qurilmalardagi sessiyalar tugatildi.", false)
		}
	default:
		return
	}
	if err != nil {
		b.log.Info("telegram buyruq", "cmd", cmd, "chat", u.ChatID, "err", err)
	}
}
