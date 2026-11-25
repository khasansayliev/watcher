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

func tgIdent(id int64, username string) domain.Identity {
	return domain.Identity{TelegramID: id, FirstName: "Ali", Username: username}
}

// issue — botga /login yozgan foydalanuvchi uchun kod oladi.
func (e *env) issue(t *testing.T, id int64) string {
	t.Helper()
	code, _, err := e.auth.IssueLoginCode(context.Background(), tgIdent(id, "ali"))
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func TestLoginWithCode(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	code := e.issue(t, 555)

	if _, ok := e.codes.m[code]; ok {
		t.Fatal("bazada kodning o'zi emas, xeshi saqlanishi kerak")
	}

	// Foydalanuvchi kodni qulay ko'rinishda (kichik harf, defis) kiritadi.
	typed := strings.ToLower(domain.FormatLoginCode(code))
	token, u, _, err := e.auth.LoginWithCode(ctx, typed, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if u.TelegramID != 555 || u.Username != "ali" {
		t.Fatalf("user: %+v", u)
	}
	if got, err := e.auth.Authenticate(ctx, token); err != nil || got.ID != u.ID {
		t.Fatalf("sessiya ishlamadi: %v", err)
	}

	// Kod bir marta ishlaydi.
	if _, _, _, err := e.auth.LoginWithCode(ctx, code, "1.2.3.4"); err == nil {
		t.Fatal("ishlatilgan kod qayta ishlamasligi kerak")
	}
}

func TestLoginCodeSameAccountAsWidget(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	e.verifier.ident = domain.Identity{TelegramID: 555, FirstName: "Ali", Username: "ali", AuthDate: e.clock.Now()}
	_, viaWidget, _, err := e.auth.Login(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, viaCode, _, err := e.auth.LoginWithCode(ctx, e.issue(t, 555), "ip")
	if err != nil {
		t.Fatal(err)
	}
	if viaWidget.ID != viaCode.ID {
		t.Fatal("widget va kod bilan kirish bitta akkauntga olib kelishi kerak (kuzatuvlar saqlanadi)")
	}
}

func TestLoginCodeExpiry(t *testing.T) {
	e := newEnv(10, 0)
	code := e.issue(t, 555)
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
	old := e.issue(t, 555)
	e.clock.add(domain.LoginCodeCooldown + time.Second)
	fresh := e.issue(t, 555)
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

func TestIssueLoginCodeCooldown(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	e.issue(t, 555)
	var ve *domain.ValidationError
	if _, _, err := e.auth.IssueLoginCode(ctx, tgIdent(555, "ali")); !errors.As(err, &ve) {
		t.Fatalf("tez-tez kod so'rash cheklanishi kerak: %v", err)
	}
	// Boshqa akkauntga ta'sir qilmaydi.
	if _, _, err := e.auth.IssueLoginCode(ctx, tgIdent(556, "vali")); err != nil {
		t.Fatal(err)
	}
	e.clock.add(domain.LoginCodeCooldown)
	if _, _, err := e.auth.IssueLoginCode(ctx, tgIdent(555, "ali")); err != nil {
		t.Fatalf("oraliq o'tgach yana berilishi kerak: %v", err)
	}
	if _, _, err := e.auth.IssueLoginCode(ctx, domain.Identity{}); err == nil {
		t.Fatal("Telegram ID'siz kod berilmasligi kerak")
	}
}

func TestLoginCodeBruteForceLimit(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	code := e.issue(t, 555)

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
	code := e.issue(t, 555)
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
	if _, _, _, err := e.auth.LoginWithCode(ctx, e.issue(t, 555), "ip"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if _, _, _, err := e.auth.LoginWithCode(ctx, "AAAAAAAA", "ip"); errors.Is(err, usecase.ErrTooManyAttempts) {
			t.Fatal("muvaffaqiyatli kirishdan keyin hisob nolga tushishi kerak")
		}
	}
}

func TestSendLoginCodeMessage(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	if err := e.tg.SendLoginCode(ctx, tgIdent(555, "ali")); err != nil {
		t.Fatal(err)
	}
	if e.notif.count("555") != 1 {
		t.Fatal("kod faqat so'ragan foydalanuvchining chatiga yuborilishi kerak")
	}
	msg := e.notif.sent[0].text
	code := codeRe.FindString(msg)
	if code == "" || !strings.Contains(msg, "https://watcher.test/login") || !strings.Contains(msg, "5 daqiqa") {
		t.Fatalf("xabar: %q", msg)
	}
	if _, _, _, err := e.auth.LoginWithCode(ctx, code, "ip"); err != nil {
		t.Fatalf("xabardagi kod ishlashi kerak: %v", err)
	}

	// Tez-tez so'ralsa kod emas, sabab yuboriladi.
	_ = e.tg.SendLoginCode(ctx, tgIdent(556, "vali"))
	_ = e.tg.SendLoginCode(ctx, tgIdent(556, "vali"))
	if e.notif.count("556") != 2 || codeRe.MatchString(e.notif.sent[2].text) {
		t.Fatalf("ikkinchi xabarda kod bo'lmasligi kerak: %+v", e.notif.sent)
	}
}

func TestCleanupRemovesExpiredCodes(t *testing.T) {
	e := newEnv(10, 0)
	e.issue(t, 555)
	e.clock.add(domain.LoginCodeTTL + time.Minute)
	if _, err := e.auth.CleanupExpired(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.codes.m) != 0 {
		t.Fatal("eskirgan kodlar tozalanishi kerak")
	}
}
