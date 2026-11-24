package telegram

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/khasansayliev/watcher/internal/usecase"
)

// signRef — Telegram hujjatidagi algoritmning mustaqil yozilgan nusxasi (testdagi "haqiqat manbai").
func signRef(token, dataCheckString string) string {
	secret := sha256.Sum256([]byte(token))
	m := hmac.New(sha256.New, secret[:])
	m.Write([]byte(dataCheckString))
	return hex.EncodeToString(m.Sum(nil))
}

func TestLoginVerifier(t *testing.T) {
	const token = "123456:TEST-token"
	fields := map[string]string{
		"id": "987654321", "first_name": "Jem", "last_name": "Torayev", "username": "jem_t",
		"photo_url": "https://t.me/i/userpic/320/x.jpg", "auth_date": "1795410000",
	}
	// Kalitlar alifbo tartibida, "\n" bilan.
	dcs := "auth_date=1795410000\nfirst_name=Jem\nid=987654321\nlast_name=Torayev\nphoto_url=https://t.me/i/userpic/320/x.jpg\nusername=jem_t"
	fields["hash"] = signRef(token, dcs)

	v := NewLoginVerifier(token)
	id, err := v.Verify(fields)
	if err != nil {
		t.Fatal(err)
	}
	if id.TelegramID != 987654321 || id.Username != "jem_t" || id.FullName() != "Jem Torayev" || id.AuthDate.Unix() != 1795410000 {
		t.Fatalf("identity: %+v", id)
	}

	cases := map[string]func(map[string]string){
		"boshqa token bilan imzolangan": func(f map[string]string) { f["hash"] = signRef("999:other", dcs) },
		"id almashtirilgan":             func(f map[string]string) { f["id"] = "1" },
		"maydon qo'shilgan":             func(f map[string]string) { f["is_admin"] = "true" },
		"hash yo'q":                     func(f map[string]string) { delete(f, "hash") },
	}
	for name, mutate := range cases {
		f := map[string]string{}
		for k, v := range fields {
			f[k] = v
		}
		mutate(f)
		if _, err := v.Verify(f); !errors.Is(err, usecase.ErrBadSignature) {
			t.Errorf("%s: ErrBadSignature kutilgan, olindi %v", name, err)
		}
	}
}
