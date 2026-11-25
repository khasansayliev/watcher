package domain

import (
	"crypto/rand"
	"strings"
	"time"
)

// Kod bilan kirish: Telegram'i bor qurilmada (telefon) botga /login yoziladi, bot bir martalik kod beradi,
// kod Telegram'i yo'q qurilmadagi saytga kiritiladi.
//
// Kod 8 ta belgi, 32 belgilik alifbodan: 32^8 = 2^40 variant. Chalkash belgilar (0/O, 1/I) alifboda yo'q.
const (
	LoginCodeLen = 8
	LoginCodeTTL = 5 * time.Minute
	// LoginCodeCooldown — bir Telegram akkaunt uchun yangi kod so'rash oralig'i.
	LoginCodeCooldown = 15 * time.Second

	loginCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // aynan 32 belgi: bayt&31 bir tekis taqsimlanadi
)

// NewLoginCode kriptografik tasodifiy kod yaratadi (normallashgan ko'rinishda, defissiz).
func NewLoginCode() (string, error) {
	b := make([]byte, LoginCodeLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = loginCodeAlphabet[b[i]&31]
	}
	return string(b), nil
}

// FormatLoginCode kodni o'qishga qulay ko'rinishga keltiradi: ABCD-EFGH.
func FormatLoginCode(code string) string {
	if len(code) != LoginCodeLen {
		return code
	}
	return code[:LoginCodeLen/2] + "-" + code[LoginCodeLen/2:]
}

// NormalizeLoginCode foydalanuvchi kiritgan matnni (kichik harf, bo'shliq, defis) kanonik ko'rinishga keltiradi.
// Shakli yaroqsiz bo'lsa ok=false: bunday kod bazadan qidirilmaydi ham.
func NormalizeLoginCode(s string) (code string, ok bool) {
	s = strings.ToUpper(s)
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '_', '\t', ' ':
			return -1
		}
		return r
	}, s)
	if len(s) != LoginCodeLen {
		return "", false
	}
	for _, r := range s {
		if !strings.ContainsRune(loginCodeAlphabet, r) {
			return "", false
		}
	}
	return s, true
}
