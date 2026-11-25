package usecase

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
)

// ErrBotBlocked — bot foydalanuvchiga yoza olmaydi (botni bloklagan yoki yozish ruxsatini bermagan).
var ErrBotBlocked = errors.New("bot foydalanuvchiga yoza olmaydi")

// TelegramService — botning foydalanuvchi bilan bevosita muloqoti.
// Akkaunt Telegram orqali ochilgani uchun alohida "ulash" bosqichi yo'q:
// bildirishnomalar foydalanuvchining o'z Telegram ID'siga (bot bilan shaxsiy chat) yuboriladi.
type TelegramService struct {
	users    UserRepository
	notifier Notifier
	codes    LoginCodeIssuer
	siteURL  string
}

// LoginCodeIssuer — kod bilan kirish uchun bir martalik kod beradi (AuthService amalga oshiradi).
type LoginCodeIssuer interface {
	IssueLoginCode(ctx context.Context, ident domain.Identity) (code string, ttl time.Duration, err error)
}

func NewTelegramService(users UserRepository, n Notifier, codes LoginCodeIssuer, siteURL string) *TelegramService {
	return &TelegramService{users: users, notifier: n, codes: codes, siteURL: siteURL}
}

// Greet — botga /start yozganlarga yo'riqnoma.
func (s *TelegramService) Greet(ctx context.Context, chatID string) error {
	return s.notifier.SendText(ctx, chatID, "Salom! Bu Watcher boti. "+s.siteURL+
		" saytiga \"Telegram orqali kirish\" tugmasi bilan kiring va kuzatuv yarating — poyezdga bo'sh joy chiqsa shu yerga xabar keladi.\n\n"+
		"Qurilmangizda Telegram yo'qmi? Shu yerga /login yozing — sayt uchun bir martalik kod beraman.")
}

// SendLoginCode foydalanuvchining o'z Telegram chatiga bir martalik kirish kodini yuboradi.
// Shartlar buzilsa (juda tez-tez so'rash) sababi shu chatga yoziladi.
func (s *TelegramService) SendLoginCode(ctx context.Context, ident domain.Identity) error {
	chat := strconv.FormatInt(ident.TelegramID, 10)
	code, ttl, err := s.codes.IssueLoginCode(ctx, ident)
	if err != nil {
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			return s.notifier.SendText(ctx, chat, ve.Msg)
		}
		return err
	}
	return s.notifier.SendText(ctx, chat, "🔐 Kirish kodi: "+domain.FormatLoginCode(code)+"\n\n"+
		s.siteURL+"/login sahifasidagi \"Kod bilan kirish\" maydoniga kiriting.\n"+
		"Kod "+strconv.Itoa(int(ttl.Minutes()))+" daqiqa amal qiladi va faqat bir marta ishlaydi.\n\n"+
		"⚠️ Kodni hech kimga bermang. Bu so'rovni siz yubormagan bo'lsangiz, e'tibor bermang.")
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
