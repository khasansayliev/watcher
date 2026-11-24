package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
)

<<<<<<< Updated upstream
// ErrBadSignature — Telegram imzosi mos kelmadi (ma'lumot soxta yoki buzilgan).
var ErrBadSignature = errors.New("telegram imzosi noto'g'ri")

// AuthService — Telegram orqali kirish va sessiyalar.
type AuthService struct {
	verifier IdentityVerifier
=======
const loginRequestTTL = 10 * time.Minute

// AuthService — Telegram orqali kirish/ro'yxatdan o'tish va sessiyalar.
//
// Oqim: brauzer BeginLogin bilan bir martalik kod oladi (u cookie'da qoladi) →
// foydalanuvchi t.me/<bot>?start=<kod> ni ochib Start bosadi → bot ConfirmLogin'ni chaqiradi
// (yangi bo'lsa akkaunt yaratiladi) → brauzer PollLogin orqali sessiya tokenini oladi.
// Parol yo'q; telefon/akkaunt egaligi Telegram tomonidan tasdiqlanadi.
type AuthService struct {
>>>>>>> Stashed changes
	users    UserRepository
	logins   LoginRequestRepository
	sessions SessionRepository
	notifier Notifier
	clock    Clock
	ttl      time.Duration
}

<<<<<<< Updated upstream
func NewAuthService(v IdentityVerifier, users UserRepository, sessions SessionRepository, c Clock, ttl time.Duration) *AuthService {
	return &AuthService{verifier: v, users: users, sessions: sessions, clock: c, ttl: ttl}
}

// Login Telegram Login Widget qaytargan maydonlarni tekshiradi va sessiya ochadi.
// Token faqat foydalanuvchiga (cookie) beriladi; bazada uning sha256 xeshi saqlanadi.
func (s *AuthService) Login(ctx context.Context, fields map[string]string) (token string, user domain.User, expires time.Time, err error) {
	ident, err := s.verifier.Verify(fields)
	if err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	now := s.clock.Now()
	if err := ident.Validate(now); err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	user, err = s.users.UpsertByIdentity(ctx, ident, now)
=======
func NewAuthService(users UserRepository, logins LoginRequestRepository, sessions SessionRepository, n Notifier, c Clock, ttl time.Duration) *AuthService {
	return &AuthService{users: users, logins: logins, sessions: sessions, notifier: n, clock: c, ttl: ttl}
}

// BeginLogin yangi kirish so'rovi ochadi: brauzerga beriladigan kod va botni ochuvchi havola.
func (s *AuthService) BeginLogin(ctx context.Context) (code, botURL string, err error) {
	if !s.notifier.Enabled() {
		return "", "", domain.NewValidationError("Telegram bot sozlanmagan (TELEGRAM_BOT_TOKEN)")
	}
	code, err = RandomToken(18) // 24 belgi; Telegram start parametri 64 belgigacha, [A-Za-z0-9_-]
	if err != nil {
		return "", "", err
	}
	if err := s.logins.Begin(ctx, code, s.clock.Now().Add(loginRequestTTL)); err != nil {
		return "", "", err
	}
	botURL, err = s.notifier.BotURL(code)
	return code, botURL, err
}

// BotURL mavjud kod uchun botni ochuvchi havolani qaytaradi (kutish sahifasi uchun).
func (s *AuthService) BotURL(code string) (string, error) { return s.notifier.BotURL(code) }

// ConfirmLogin botdan kelgan "/start <kod>" ni qayta ishlaydi: profilni yaratadi/yangilaydi
// va so'rovni shu foydalanuvchiga bog'laydi.
func (s *AuthService) ConfirmLogin(ctx context.Context, code string, p domain.TelegramProfile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	now := s.clock.Now()
	user, err := s.users.UpsertByTelegram(ctx, p, now)
	if err != nil {
		return err
	}
	if err := s.logins.Approve(ctx, code, user.ID, now); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			_ = s.notifier.SendText(ctx, p.ID, "Havola eskirgan yoki ishlatilgan. Saytda qaytadan \"Telegram orqali kirish\" tugmasini bosing.")
		}
		return err
	}
	return s.notifier.SendText(ctx, p.ID, "✅ Kirish tasdiqlandi, "+user.DisplayName()+". Saytga qayting — sahifa o'zi yangilanadi.\n\nBo'sh joy chiqsa xabar shu chatga keladi. Agar bu siz emas bo'lsangiz, e'tibor bermang va /logout yozing.")
}

// Greet — kodsiz /start yozganlarga yo'riqnoma.
func (s *AuthService) Greet(ctx context.Context, chatID string) error {
	return s.notifier.SendText(ctx, chatID, "Salom! Saytga kirish uchun saytdagi \"Telegram orqali kirish\" tugmasini bosing, so'ng bu yerga qaytib Start bosing.")
}

// PollLogin brauzer kodining tasdiqlanganini tekshiradi; tasdiqlangan bo'lsa sessiya ochadi.
// Token faqat brauzerga (cookie) beriladi; bazada uning sha256 xeshi saqlanadi.
// Hali tasdiqlanmagan bo'lsa ErrLoginPending, kod yaroqsiz/eskirgan bo'lsa domain.ErrNotFound.
func (s *AuthService) PollLogin(ctx context.Context, code string) (token string, user domain.User, expires time.Time, err error) {
	if code == "" {
		return "", domain.User{}, time.Time{}, domain.ErrNotFound
	}
	now := s.clock.Now()
	userID, err := s.logins.Claim(ctx, code, now)
>>>>>>> Stashed changes
	if err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	if user, err = s.users.Get(ctx, userID); err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	if token, err = RandomToken(32); err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	expires = now.Add(s.ttl)
	if err := s.sessions.Create(ctx, HashToken(token), user.ID, expires); err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	return token, user, expires, nil
}

// Authenticate cookie'dagi token egasini qaytaradi.
func (s *AuthService) Authenticate(ctx context.Context, token string) (domain.User, error) {
	if token == "" {
		return domain.User{}, domain.ErrUnauthorized
	}
	u, err := s.sessions.UserByToken(ctx, HashToken(token), s.clock.Now())
	if errors.Is(err, domain.ErrNotFound) {
		return domain.User{}, domain.ErrUnauthorized
	}
	return u, err
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.sessions.Delete(ctx, HashToken(token))
}

// LogoutAll foydalanuvchining barcha sessiyalarini tugatadi (botdagi /logout).
func (s *AuthService) LogoutAll(ctx context.Context, telegramID string) error {
	return s.sessions.DeleteByTelegram(ctx, telegramID)
}

// CleanupExpired eskirgan sessiyalar va kirish so'rovlarini o'chiradi (davriy chaqiriladi).
func (s *AuthService) CleanupExpired(ctx context.Context) (int64, error) {
	return s.sessions.DeleteExpired(ctx, s.clock.Now())
}

// RandomToken — URL uchun xavfsiz tasodifiy satr.
func RandomToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func HashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}
