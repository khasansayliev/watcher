package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
)

const loginRequestTTL = 10 * time.Minute

// ErrTooManyAttempts — kod bilan kirishda juda ko'p noto'g'ri urinish bo'ldi.
var ErrTooManyAttempts = errors.New("juda ko'p noto'g'ri urinish")

const (
	codeAttemptWindow = 10 * time.Minute
	codeAttemptsPerIP = 5  // bitta manzildan oynada
	codeAttemptsTotal = 60 // hamma manzillardan oynada
)

// AuthService — Telegram orqali kirish/ro'yxatdan o'tish va sessiyalar.
//
// Oqim: brauzer BeginLogin bilan bir martalik kod oladi (u cookie'da qoladi) →
// foydalanuvchi t.me/<bot>?start=<kod> ni ochib Start bosadi → bot ConfirmLogin'ni chaqiradi
// (yangi bo'lsa akkaunt yaratiladi) → brauzer PollLogin orqali sessiya tokenini oladi.
// Parol yo'q; telefon/akkaunt egaligi Telegram tomonidan tasdiqlanadi.
//
// Qo'shimcha oqim (Telegram o'rnatilmagan qurilma uchun): foydalanuvchi boshqa qurilmada (telefon) botga /login
// yozadi → bot SendLoginCode bilan bir martalik kod yuboradi → foydalanuvchi kodni saytga kiritadi → LoginWithCode.
type AuthService struct {
	users    UserRepository
	logins   LoginRequestRepository
	sessions SessionRepository
	codes    LoginCodeRepository
	notifier Notifier
	clock    Clock
	ttl      time.Duration
	siteURL  string

	attempts *attemptLimiter
	issued   *cooldown
}

func NewAuthService(users UserRepository, logins LoginRequestRepository, sessions SessionRepository, codes LoginCodeRepository, n Notifier, c Clock, ttl time.Duration) *AuthService {
	return &AuthService{
		users: users, logins: logins, sessions: sessions, codes: codes, notifier: n, clock: c, ttl: ttl,
		attempts: newAttemptLimiter(codeAttemptWindow, codeAttemptsPerIP, codeAttemptsTotal),
		issued:   newCooldown(),
	}
}

// WithSiteURL bot yuboradigan xabarlarda saytning manzilini ko'rsatish uchun.
func (s *AuthService) WithSiteURL(u string) *AuthService {
	s.siteURL = strings.TrimRight(u, "/")
	return s
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
	if user.Disabled { // brauzer PollLogin'da rad etiladi; bu yerda sababini aytamiz
		return s.notifier.SendText(ctx, p.ID, "⛔ Akkauntingiz administrator tomonidan o'chirilgan. Kirish mumkin emas.")
	}
	return s.notifier.SendText(ctx, p.ID, "✅ Kirish tasdiqlandi, "+user.DisplayName()+". Saytga qayting — sahifa o'zi yangilanadi.\n\nBo'sh joy chiqsa xabar shu chatga keladi. Agar bu siz emas bo'lsangiz, e'tibor bermang va /logout yozing.")
}

// Greet — kodsiz /start yozganlarga yo'riqnoma.
func (s *AuthService) Greet(ctx context.Context, chatID string) error {
	return s.notifier.SendText(ctx, chatID, "Salom! Saytga kirish uchun saytdagi \"Telegram orqali kirish\" tugmasini bosing, so'ng bu yerga qaytib Start bosing.\n\n"+
		"Telegram o'rnatilmagan qurilmadan kirmoqchimisiz? Shu yerga /login yozing — saytga kiritish uchun bir martalik kod beraman.")
}

// SendLoginCode botga /login yozgan (Telegram tasdiqlagan) foydalanuvchiga bir martalik kod yuboradi.
// Kod faqat o'sha chatga ketadi; bazada xeshi saqlanadi; oldingi kodlari bekor bo'ladi.
// Juda tez-tez so'ralsa kod emas, sababi yoziladi.
func (s *AuthService) SendLoginCode(ctx context.Context, p domain.TelegramProfile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	now := s.clock.Now()
	if !s.issued.try(p.ID, now, domain.LoginCodeCooldown) {
		return s.notifier.SendText(ctx, p.ID, "Yangi kod uchun bir necha soniya kuting va qaytadan /login yozing.")
	}
	code, err := domain.NewLoginCode()
	if err != nil {
		return err
	}
	if err := s.codes.Put(ctx, HashToken(code), p, now.Add(domain.LoginCodeTTL)); err != nil {
		return err
	}
	where := "saytdagi \"Kod bilan kirish\" maydoniga"
	if s.siteURL != "" {
		where = s.siteURL + "/login sahifasidagi \"Kod bilan kirish\" maydoniga"
	}
	return s.notifier.SendText(ctx, p.ID, "🔐 Kirish kodi: "+domain.FormatLoginCode(code)+"\n\n"+
		"Kodni "+where+" kiriting.\n"+
		"Kod "+strconv.Itoa(int(domain.LoginCodeTTL.Minutes()))+" daqiqa amal qiladi va faqat bir marta ishlaydi.\n\n"+
		"⚠️ Kodni hech kimga bermang. Bu so'rovni siz yubormagan bo'lsangiz, e'tibor bermang.")
}

// LoginWithCode saytga kiritilgan kodni tekshiradi va sessiya ochadi (akkaunt yo'q bo'lsa yaratiladi).
// Kod bir marta ishlaydi. clientKey — urinishlarni cheklash uchun so'rov egasi (odatda IP).
func (s *AuthService) LoginWithCode(ctx context.Context, rawCode, clientKey string) (token string, user domain.User, expires time.Time, err error) {
	now := s.clock.Now()
	if !s.attempts.allowed(clientKey, now) {
		return "", domain.User{}, time.Time{}, ErrTooManyAttempts
	}
	badCode := domain.NewValidationError("Kod noto'g'ri yoki muddati o'tgan. Botga /login yozib yangi kod oling")

	code, ok := domain.NormalizeLoginCode(rawCode)
	if !ok {
		s.attempts.fail(clientKey, now)
		return "", domain.User{}, time.Time{}, badCode
	}
	p, err := s.codes.Consume(ctx, HashToken(code), now)
	if errors.Is(err, domain.ErrNotFound) {
		s.attempts.fail(clientKey, now)
		return "", domain.User{}, time.Time{}, badCode
	}
	if err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	s.attempts.reset(clientKey)

	if user, err = s.users.UpsertByTelegram(ctx, p, now); err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	if user.Disabled {
		return "", domain.User{}, time.Time{}, domain.ErrForbidden
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

// PollLogin brauzer kodining tasdiqlanganini tekshiradi; tasdiqlangan bo'lsa sessiya ochadi.
// Token faqat brauzerga (cookie) beriladi; bazada uning sha256 xeshi saqlanadi.
// Hali tasdiqlanmagan bo'lsa ErrLoginPending, kod yaroqsiz/eskirgan bo'lsa domain.ErrNotFound.
func (s *AuthService) PollLogin(ctx context.Context, code string) (token string, user domain.User, expires time.Time, err error) {
	if code == "" {
		return "", domain.User{}, time.Time{}, domain.ErrNotFound
	}
	now := s.clock.Now()
	userID, err := s.logins.Claim(ctx, code, now)
	if err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	if user, err = s.users.Get(ctx, userID); err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	if user.Disabled {
		return "", domain.User{}, time.Time{}, domain.ErrForbidden
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
	if err == nil && u.Disabled {
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

// CleanupExpired eskirgan sessiyalar, kirish so'rovlari va kodlarni o'chiradi (davriy chaqiriladi).
func (s *AuthService) CleanupExpired(ctx context.Context) (int64, error) {
	now := s.clock.Now()
	if _, err := s.codes.DeleteExpired(ctx, now); err != nil {
		return 0, err
	}
	return s.sessions.DeleteExpired(ctx, now)
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
