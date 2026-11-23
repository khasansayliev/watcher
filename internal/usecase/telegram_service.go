package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
)

const linkCodeTTL = 15 * time.Minute

// TelegramService — foydalanuvchi akkauntini Telegram chatiga bog'lash.
//
// Oqim: UI'da "Ulash" → bir martalik kod yaratiladi → t.me/<bot>?start=<kod> ochiladi →
// foydalanuvchi Start bosadi → bot "/start <kod>" oladi → LinkChat chat ID'ni akkauntga bog'laydi.
// Shunday qilib hech kim boshqa birovning chat ID'sini qo'lda kirita olmaydi.
type TelegramService struct {
	users    UserRepository
	codes    LinkCodeRepository
	notifier Notifier
	clock    Clock
}

func NewTelegramService(users UserRepository, codes LinkCodeRepository, n Notifier, c Clock) *TelegramService {
	return &TelegramService{users: users, codes: codes, notifier: n, clock: c}
}

// StartLink bir martalik kod yaratadi va botni ochuvchi havolani qaytaradi.
func (s *TelegramService) StartLink(ctx context.Context, userID int64) (string, error) {
	if !s.notifier.Enabled() {
		return "", domain.NewValidationError("Telegram bot sozlanmagan (TELEGRAM_BOT_TOKEN)")
	}
	code, err := RandomToken(18) // 24 belgi; Telegram start parametri 64 belgigacha
	if err != nil {
		return "", err
	}
	if err := s.codes.Create(ctx, code, userID, s.clock.Now().Add(linkCodeTTL)); err != nil {
		return "", err
	}
	return s.notifier.LinkURL(code)
}

// LinkChat botdan kelgan "/start <kod>" ni qayta ishlaydi.
func (s *TelegramService) LinkChat(ctx context.Context, code, chatID string) error {
	userID, err := s.codes.Consume(ctx, code, s.clock.Now())
	if errors.Is(err, domain.ErrNotFound) {
		_ = s.notifier.SendText(ctx, chatID, "Havola eskirgan yoki ishlatilgan. Saytdagi Sozlamalar sahifasidan qaytadan \"Telegram'ni ulash\" ni bosing.")
		return err
	}
	if err != nil {
		return err
	}
	if err := s.users.SetTelegramChat(ctx, userID, chatID); err != nil {
		return err
	}
	u, err := s.users.Get(ctx, userID)
	if err != nil {
		return err
	}
	return s.notifier.SendText(ctx, chatID, "✅ Telegram "+u.Email+" akkauntiga ulandi. Bo'sh joy chiqsa shu yerga xabar keladi.")
}

// Greet — kodsiz /start yozganlarga yo'riqnoma.
func (s *TelegramService) Greet(ctx context.Context, chatID string) error {
	return s.notifier.SendText(ctx, chatID, "Salom! Bildirishnomalarni olish uchun saytga Google orqali kiring va Sozlamalar sahifasida \"Telegram'ni ulash\" tugmasini bosing.")
}

func (s *TelegramService) Unlink(ctx context.Context, userID int64) error {
	return s.users.SetTelegramChat(ctx, userID, "")
}

func (s *TelegramService) SendTest(ctx context.Context, userID int64) error {
	u, err := s.users.Get(ctx, userID)
	if err != nil {
		return err
	}
	if !u.HasTelegram() {
		return domain.NewValidationError("Telegram ulanmagan")
	}
	return s.notifier.SendText(ctx, u.TelegramChatID, "✅ Test xabar: Watcher ishlayapti.")
}
