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

// ErrBadSignature — Telegram imzosi mos kelmadi (ma'lumot soxta yoki buzilgan).
var ErrBadSignature = errors.New("telegram imzosi noto'g'ri")

// ErrTooManyAttempts — kod bilan kirishda juda ko'p noto'g'ri urinish bo'ldi.
var ErrTooManyAttempts = errors.New("juda ko'p noto'g'ri urinish")

const (
	codeAttemptWindow = 10 * time.Minute
	codeAttemptsPerIP = 5  // bitta manzildan oynada
	codeAttemptsTotal = 60 // hamma manzillardan oynada
)

// AuthService — Telegram orqali kirish (widget yoki bot bergan kod) va sessiyalar.
type AuthService struct {
	verifier IdentityVerifier
	users    UserRepository
	sessions SessionRepository
	codes    LoginCodeRepository
	clock    Clock
	ttl      time.Duration

	attempts *attemptLimiter
	issued   *cooldown
}

func NewAuthService(v IdentityVerifier, users UserRepository, sessions SessionRepository, codes LoginCodeRepository, c Clock, ttl time.Duration) *AuthService {
	return &AuthService{
		verifier: v, users: users, sessions: sessions, codes: codes, clock: c, ttl: ttl,
		attempts: newAttemptLimiter(codeAttemptWindow, codeAttemptsPerIP, codeAttemptsTotal),
		issued:   newCooldown(),
	}
}

// Login Telegram Login Widget qaytargan maydonlarni tekshiradi va sessiya ochadi.
// Token faqat foydalanuvchiga (cookie) beriladi; bazada uning sha256 xeshi saqlanadi.
func (s *AuthService) Login(ctx context.Context, fields map[string]string) (token string, user domain.User, expires time.Time, err error) {
	ident, err := s.verifier.Verify(fields)
	if err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	return s.startSession(ctx, ident)
}

// IssueLoginCode botga /login yozgan (Telegram tasdiqlagan) foydalanuvchiga bir martalik kod beradi.
// Kod faqat Telegram chatiga yuboriladi; bazada xeshi saqlanadi. Eski kodlari bekor bo'ladi.
func (s *AuthService) IssueLoginCode(ctx context.Context, ident domain.Identity) (code string, ttl time.Duration, err error) {
	now := s.clock.Now()
	ident.AuthDate = now
	if err := ident.Validate(now); err != nil {
		return "", 0, err
	}
	if !s.issued.try(ident.TelegramID, now, domain.LoginCodeCooldown) {
		return "", 0, domain.NewValidationError("Yangi kod uchun bir necha soniya kuting va qaytadan /login yozing")
	}
	code, err = domain.NewLoginCode()
	if err != nil {
		return "", 0, err
	}
	if err := s.codes.Put(ctx, HashToken(code), ident, now.Add(domain.LoginCodeTTL)); err != nil {
		return "", 0, err
	}
	return code, domain.LoginCodeTTL, nil
}

// LoginWithCode saytga kiritilgan kodni tekshiradi va sessiya ochadi. Kod bir marta ishlaydi.
// clientKey — urinishlarni cheklash uchun so'rov egasi (odatda IP).
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
	ident, err := s.codes.Consume(ctx, HashToken(code), now)
	if errors.Is(err, domain.ErrNotFound) {
		s.attempts.fail(clientKey, now)
		return "", domain.User{}, time.Time{}, badCode
	}
	if err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	s.attempts.reset(clientKey)
	ident.AuthDate = now // kod hozirgina tasdiqlandi
	return s.startSession(ctx, ident)
}

func (s *AuthService) startSession(ctx context.Context, ident domain.Identity) (token string, user domain.User, expires time.Time, err error) {
	now := s.clock.Now()
	if err := ident.Validate(now); err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	user, err = s.users.UpsertByIdentity(ctx, ident, now)
	if err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	token, err = RandomToken(32)
	if err != nil {
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

// CleanupExpired eskirgan sessiyalar va kodlarni o'chiradi (davriy chaqiriladi).
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
