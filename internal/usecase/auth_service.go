package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
)

// AuthService — Google orqali kirish va sessiyalar.
type AuthService struct {
	idp      IdentityProvider
	users    UserRepository
	sessions SessionRepository
	clock    Clock
	ttl      time.Duration
}

func NewAuthService(idp IdentityProvider, users UserRepository, sessions SessionRepository, c Clock, ttl time.Duration) *AuthService {
	return &AuthService{idp: idp, users: users, sessions: sessions, clock: c, ttl: ttl}
}

// LoginURL Google'ning kirish sahifasi havolasi. state — CSRF himoyasi uchun tasodifiy qiymat.
func (s *AuthService) LoginURL(state string) string { return s.idp.AuthURL(state) }

// Login Google qaytargan kodni sessiya tokeniga almashtiradi.
// Token faqat foydalanuvchiga (cookie) beriladi; bazada uning sha256 xeshi saqlanadi.
func (s *AuthService) Login(ctx context.Context, code string) (token string, user domain.User, expires time.Time, err error) {
	ident, err := s.idp.Exchange(ctx, code)
	if err != nil {
		return "", domain.User{}, time.Time{}, fmt.Errorf("google: %w", err)
	}
	if err := ident.Validate(); err != nil {
		return "", domain.User{}, time.Time{}, err
	}
	now := s.clock.Now()
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

// CleanupExpired eskirgan sessiyalarni o'chiradi (davriy chaqiriladi).
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
