package usecase

import (
	"context"

	"github.com/khasansayliev/watcher/internal/domain"
)

// TelegramService — foydalanuvchiga Telegram orqali xabar yuborish (akkaunt va chat bir xil,
// shuning uchun alohida "ulash" bosqichi yo'q).
type TelegramService struct {
	users    UserRepository
	notifier Notifier
}

func NewTelegramService(users UserRepository, n Notifier) *TelegramService {
	return &TelegramService{users: users, notifier: n}
}

func (s *TelegramService) SendTest(ctx context.Context, userID int64) error {
	u, err := s.users.Get(ctx, userID)
	if err != nil {
		return err
	}
	if !u.HasTelegram() {
		return domain.NewValidationError("Telegram akkaunt topilmadi")
	}
	return s.notifier.SendText(ctx, u.TelegramChatID, "✅ Test xabar: Watcher ishlayapti.")
}
