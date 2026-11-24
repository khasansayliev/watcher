package telegram

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

// LoginVerifier — Telegram Login Widget ma'lumotlarining imzosini tekshiradi (usecase.IdentityVerifier).
// https://core.telegram.org/widgets/login#checking-authorization
//
//	secret_key        = SHA256(bot_token)
//	data_check_string = hash'dan boshqa barcha maydonlar, kalit bo'yicha tartiblangan, "key=value", "\n" bilan
//	hash              = hex(HMAC_SHA256(data_check_string, secret_key))
type LoginVerifier struct {
	secret []byte
}

var _ usecase.IdentityVerifier = (*LoginVerifier)(nil)

func NewLoginVerifier(botToken string) *LoginVerifier {
	s := sha256.Sum256([]byte(botToken))
	return &LoginVerifier{secret: s[:]}
}

func (v *LoginVerifier) Verify(fields map[string]string) (domain.Identity, error) {
	got := fields["hash"]
	if got == "" {
		return domain.Identity{}, usecase.ErrBadSignature
	}
	if !hmac.Equal([]byte(strings.ToLower(got)), []byte(v.sign(fields))) {
		return domain.Identity{}, usecase.ErrBadSignature
	}

	id, err := strconv.ParseInt(fields["id"], 10, 64)
	if err != nil {
		return domain.Identity{}, fmt.Errorf("telegram: id noto'g'ri: %w", usecase.ErrBadSignature)
	}
	authUnix, err := strconv.ParseInt(fields["auth_date"], 10, 64)
	if err != nil {
		return domain.Identity{}, fmt.Errorf("telegram: auth_date noto'g'ri: %w", usecase.ErrBadSignature)
	}
	return domain.Identity{
		TelegramID: id,
		FirstName:  fields["first_name"],
		LastName:   fields["last_name"],
		Username:   fields["username"],
		PhotoURL:   fields["photo_url"],
		AuthDate:   time.Unix(authUnix, 0),
	}, nil
}

// sign — maydonlar uchun kutilgan hash (hex).
func (v *LoginVerifier) sign(fields map[string]string) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if k != "hash" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k + "=" + fields[k]
	}
	mac := hmac.New(sha256.New, v.secret)
	mac.Write([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(mac.Sum(nil))
}
