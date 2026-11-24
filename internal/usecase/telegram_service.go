package usecase

import (
	"context"
	"errors"
	"strings"
)

// ErrBotBlocked — bot foydalanuvchiga yoza olmaydi (botni bloklagan yoki yozish ruxsatini bermagan).
var ErrBotBlocked = errors.New("bot foydalanuvchiga yoza olmaydi")

// TelegramService — botning foydalanuvchi bilan bevosita muloqoti.
// Akkaunt Telegram orqali ochilgani uchun alohida "ulash" bosqichi yo'q:
// bildirishnomalar foydalanuvchining o'z Telegram ID'siga (bot bilan shaxsiy chat) yuboriladi.
type TelegramService struct {
	users    UserRepository
	notifier Notifier
	siteURL  string
}

func NewTelegramService(users UserRepository, n Notifier, siteURL string) *TelegramService {
	return &TelegramService{users: users, notifier: n, siteURL: siteURL}
}

// Greet — botga /start yozganlarga yo'riqnoma.
func (s *TelegramService) Greet(ctx context.Context, chatID string) error {
	return s.notifier.SendText(ctx, chatID, "Salom! Bu Watcher boti. "+s.siteURL+
		" saytiga \"Telegram orqali kirish\" tugmasi bilan kiring va kuzatuv yarating — poyezdga bo'sh joy chiqsa shu yerga xabar keladi.")
}

func (s *TelegramService) SendTest(ctx context.Context, userID int64) error {
	u, err := s.users.Get(ctx, userID)
	if err != nil {
		return err
	}
	err = s.notifier.SendText(ctx, u.ChatID(), "✅ Test xabar: Watcher ishlayapti.")
	if err != nil && isForbidden(err) {
		return ErrBotBlocked
	}
	return err
}

// isForbidden — Telegram "bot can't initiate conversation" / "bot was blocked by the user" xatolari.
func isForbidden(err error) bool {
	if errors.Is(err, ErrBotBlocked) {
		return true
	}
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "forbidden") || strings.Contains(m, "blocked") || strings.Contains(m, "can't initiate")
}
