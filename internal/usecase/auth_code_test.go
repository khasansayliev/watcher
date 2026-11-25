package usecase_test

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

var codeRe = regexp.MustCompile(`[A-Z2-9]{4}-[A-Z2-9]{4}`)

func prof(id, username string) domain.TelegramProfile {
	return domain.TelegramProfile{ID: id, Username: username, Name: "Ali"}
}

// sendCode — botga /login yozgan foydalanuvchi: bot kodni chatga yuboradi, biz uni xabardan o'qiymiz.
func (e *env) sendCode(t *testing.T, p domain.TelegramProfile) string {
	t.Helper()
	before := e.notif.count(p.ID)
	if err := e.auth.SendLoginCode(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if e.notif.count(p.ID) != before+1 {
		t.Fatal("bot kod yuborishi kerak edi")
	}
	e.notif.mu.Lock()
	defer e.notif.mu.Unlock()
	code := codeRe.FindString(e.notif.sent[len(e.notif.sent)-1].text)
	if code == "" {
		t.Fatalf("oxirgi xabarda kod yo'q: %q", e.notif.sent[len(e.notif.sent)-1].text)
	}
	return code
}

func TestLoginWithCode(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	code := e.sendCode(t, prof("555", "ali"))

	plain, _ := domain.NormalizeLoginCode(code)
	if _, ok := e.codes.m[plain]; ok {
		t.Fatal("bazada kodning o'zi emas, xeshi saqlanishi kerak")
	}

	// Foydalanuvchi kodni qulay ko'rinishda (kichik harf, defis) kiritadi.
	token, u, _, err := e.auth.LoginWithCode(ctx, strings.ToLower(code), "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if u.TelegramChatID != "555" || u.Username != "ali" {
		t.Fatalf("user: %+v", u)
	}
	if u.CanCreate {
		t.Fatal("yangi foydalanuvchi, tugma oqimidagi kabi, kuzatuv yaratish ruxsatisiz boshlashi kerak")
	}
	if got, err := e.auth.Authenticate(ctx, token); err != nil || got.ID != u.ID {
		t.Fatalf("sessiya ishlamadi: %v", err)
	}
	if _, ok := e.sessions.m[token]; ok {
		t.Fatal("bazada token emas, uning xeshi saqlanishi kerak")
	}

	// Kod bir marta ishlaydi.
	if _, _, _, err := e.auth.LoginWithCode(ctx, code, "1.2.3.4"); err == nil {
		t.Fatal("ishlatilgan kod qayta ishlamasligi kerak")
	}
}

func TestLoginCodeSameAccountAsButtonFlow(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	p := prof("555", "ali")

	loginCode, _, err := e.auth.BeginLogin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.auth.ConfirmLogin(ctx, loginCode, p); err != nil {
		t.Fatal(err)
	}
	_, viaButton, _, err := e.auth.PollLogin(ctx, loginCode)
	if err != nil {
		t.Fatal(err)
	}
	_, viaCode, _, err := e.auth.LoginWithCode(ctx, e.sendCode(t, p), "ip")
	if err != nil {
		t.Fatal(err)
	}
	if viaButton.ID != viaCode.ID {
		t.Fatal("tugma va kod bilan kirish bitta akkauntga olib kelishi kerak (kuzatuvlar saqlanadi)")
	}
}

func TestLoginCodeExpiry(t *testing.T) {
	e := newEnv(10, 0)
	code := e.sendCode(t, prof("555", "ali"))
	e.clock.add(domain.LoginCodeTTL + time.Second)
	_, _, _, err := e.auth.LoginWithCode(context.Background(), code, "ip")
	var ve *domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("muddati o'tgan kod rad etilishi kerak: %v", err)
	}
	if len(e.users.m) != 0 || len(e.sessions.m) != 0 {
		t.Fatal("rad etilgan urinish foydalanuvchi yoki sessiya yaratmasligi kerak")
	}
}

func TestNewCodeInvalidatesOld(t *testing.T) {
	e := newEnv(10, 0)
	p := prof("555", "ali")
	old := e.sendCode(t, p)
	e.clock.add(domain.LoginCodeCooldown + time.Second)
	fresh := e.sendCode(t, p)
	if old == fresh {
		t.Fatal("kodlar farq qilishi kerak")
	}
	if _, _, _, err := e.auth.LoginWithCode(context.Background(), old, "ip"); err == nil {
		t.Fatal("eski kod yangi kod berilgach ishlamasligi kerak")
	}
	if _, _, _, err := e.auth.LoginWithCode(context.Background(), fresh, "ip"); err != nil {
		t.Fatalf("yangi kod ishlashi kerak: %v", err)
	}
}

func TestSendLoginCodeCooldownAndMessage(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	p := prof("555", "ali")
	e.sendCode(t, p)

	msg := e.notif.sent[0].text
	if !strings.Contains(msg, "https://watcher.test/login") || !strings.Contains(msg, "5 daqiqa") || !strings.Contains(msg, "hech kimga bermang") {
		t.Fatalf("xabar: %q", msg)
	}

	// Tez-tez so'ralsa kod emas, sabab yuboriladi (ikkinchi kod berilmaydi).
	if err := e.auth.SendLoginCode(ctx, p); err != nil {
		t.Fatal(err)
	}
	if e.notif.count("555") != 2 || codeRe.MatchString(e.notif.sent[1].text) {
		t.Fatalf("ikkinchi xabarda kod bo'lmasligi kerak: %+v", e.notif.sent)
	}
	if len(e.codes.m) != 1 {
		t.Fatal("bitta akkauntda bitta amal qiluvchi kod bo'lishi kerak")
	}
	// Boshqa akkauntga ta'sir qilmaydi; oraliq o'tgach yana beriladi.
	e.sendCode(t, prof("556", "vali"))
	e.clock.add(domain.LoginCodeCooldown)
	e.sendCode(t, p)

	if err := e.auth.SendLoginCode(ctx, domain.TelegramProfile{}); err == nil {
		t.Fatal("Telegram ID'siz kod berilmasligi kerak")
	}
}

func TestLoginCodeBruteForceLimit(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	code := e.sendCode(t, prof("555", "ali"))

	for i := 0; i < 5; i++ {
		_, _, _, err := e.auth.LoginWithCode(ctx, "AAAAAAAA", "6.6.6.6")
		var ve *domain.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("urinish %d: noto'g'ri kod rad etilishi kerak: %v", i, err)
		}
	}
	// Chegara oshdi: endi to'g'ri kod ham shu manzildan o'tmaydi...
	if _, _, _, err := e.auth.LoginWithCode(ctx, code, "6.6.6.6"); !errors.Is(err, usecase.ErrTooManyAttempts) {
		t.Fatalf("manzil bloklanishi kerak: %v", err)
	}
	// ...lekin boshqa manzildan o'tadi (kod yo'qolmagan).
	if _, _, _, err := e.auth.LoginWithCode(ctx, code, "7.7.7.7"); err != nil {
		t.Fatalf("boshqa manzil bloklanmasligi kerak: %v", err)
	}
	// Vaqt o'tgach blok ochiladi.
	e.clock.add(11 * time.Minute)
	if _, _, _, err := e.auth.LoginWithCode(ctx, "AAAAAAAA", "6.6.6.6"); errors.Is(err, usecase.ErrTooManyAttempts) {
		t.Fatal("oyna o'tgach blok ochilishi kerak")
	}
}

func TestLoginCodeGlobalLimit(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	code := e.sendCode(t, prof("555", "ali"))
	// Har xil manzillardan tanlab topishga urinish: umumiy chegara to'xtatadi.
	for i := 0; i < 60; i++ {
		_, _, _, _ = e.auth.LoginWithCode(ctx, "AAAAAAAA", "ip-"+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	if _, _, _, err := e.auth.LoginWithCode(ctx, code, "yangi-manzil"); !errors.Is(err, usecase.ErrTooManyAttempts) {
		t.Fatalf("umumiy chegara ishlashi kerak: %v", err)
	}
}

func TestLoginCodeSuccessResetsFailures(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		_, _, _, _ = e.auth.LoginWithCode(ctx, "AAAAAAAA", "ip")
	}
	if _, _, _, err := e.auth.LoginWithCode(ctx, e.sendCode(t, prof("555", "ali")), "ip"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if _, _, _, err := e.auth.LoginWithCode(ctx, "AAAAAAAA", "ip"); errors.Is(err, usecase.ErrTooManyAttempts) {
			t.Fatal("muvaffaqiyatli kirishdan keyin hisob nolga tushishi kerak")
		}
	}
}

func TestLoginCodeRejectsDisabledUser(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	p := prof("555", "ali")
	_, u, _, err := e.auth.LoginWithCode(ctx, e.sendCode(t, p), "ip")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.users.SetDisabled(ctx, u.ID, true); err != nil {
		t.Fatal(err)
	}
	e.clock.add(domain.LoginCodeCooldown)
	token, _, _, err := e.auth.LoginWithCode(ctx, e.sendCode(t, p), "ip")
	if !errors.Is(err, domain.ErrForbidden) || token != "" {
		t.Fatalf("administrator o'chirgan akkaunt kod bilan kira olmasligi kerak: %q %v", token, err)
	}
}

func TestCleanupRemovesExpiredCodes(t *testing.T) {
	e := newEnv(10, 0)
	e.sendCode(t, prof("555", "ali"))
	e.clock.add(domain.LoginCodeTTL + time.Minute)
	if _, err := e.auth.CleanupExpired(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.codes.m) != 0 {
		t.Fatal("eskirgan kodlar tozalanishi kerak")
	}
}
