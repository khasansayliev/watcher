package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

func startLogin(t *testing.T, e *env, p domain.TelegramProfile) string {
	t.Helper()
	code, link, err := e.auth.BeginLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(link, "start="+code) {
		t.Fatalf("havolada kod bo'lishi kerak: %s", link)
	}
	if err := e.auth.ConfirmLogin(context.Background(), code, p); err != nil {
		t.Fatal(err)
	}
	return code
}

func TestTelegramLoginAndSession(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	prof := domain.TelegramProfile{ID: "555", Username: "ali", Name: "Ali"}

	code, _, err := e.auth.BeginLogin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := e.auth.PollLogin(ctx, code); !errors.Is(err, usecase.ErrLoginPending) {
		t.Fatalf("botda tasdiqlanmaguncha kutish kerak: %v", err)
	}
	if err := e.auth.ConfirmLogin(ctx, code, prof); err != nil {
		t.Fatal(err)
	}
	if e.notif.count("555") != 1 {
		t.Fatal("botda tasdiq xabari yuborilishi kerak")
	}

	token, u, _, err := e.auth.PollLogin(ctx, code)
	if err != nil {
		t.Fatal(err)
	}
	if u.TelegramChatID != "555" || u.Username != "ali" {
		t.Fatalf("ro'yxatdan o'tgan foydalanuvchi: %+v", u)
	}
	if len(token) < 40 {
		t.Fatalf("token juda qisqa: %q", token)
	}
	if _, ok := e.sessions.m[token]; ok {
		t.Fatal("bazada token emas, uning xeshi saqlanishi kerak")
	}
	if got, err := e.auth.Authenticate(ctx, token); err != nil || got.ID != u.ID {
		t.Fatalf("sessiya ishlamadi: %v", err)
	}
	if _, _, _, err := e.auth.PollLogin(ctx, code); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("kod faqat bir marta ishlashi kerak")
	}

	// Qayta kirish — yangi user yaratilmaydi, profil yangilanadi.
	code2 := startLogin(t, e, domain.TelegramProfile{ID: "555", Username: "ali2", Name: "Ali V"})
	_, u2, _, err := e.auth.PollLogin(ctx, code2)
	if err != nil || u2.ID != u.ID || u2.Username != "ali2" {
		t.Fatalf("bitta Telegram akkaunt — bitta foydalanuvchi: %+v %v", u2, err)
	}

	_ = e.auth.Logout(ctx, token)
	if _, err := e.auth.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("chiqqandan keyin sessiya yaroqsiz bo'lishi kerak")
	}

	token, _, _, _ = e.auth.PollLogin(ctx, startLogin(t, e, prof))
	e.clock.add(25 * time.Hour)
	if _, err := e.auth.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("muddati o'tgan sessiya rad etilishi kerak")
	}
}

func TestLoginRequestExpiresAndIsOneShot(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	prof := domain.TelegramProfile{ID: "555"}

	code, _, _ := e.auth.BeginLogin(ctx)
	e.clock.add(11 * time.Minute)
	if err := e.auth.ConfirmLogin(ctx, code, prof); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("eskirgan kod rad etilishi kerak: %v", err)
	}
	if _, _, _, err := e.auth.PollLogin(ctx, code); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("eskirgan kod bilan sessiya berilmasligi kerak")
	}

	// Tasdiqlangan kodni boshqa Telegram akkaunt qayta tasdiqlay olmaydi (kirishni o'g'irlab bo'lmaydi).
	code = startLogin(t, e, prof)
	if err := e.auth.ConfirmLogin(ctx, code, domain.TelegramProfile{ID: "666"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("kod faqat bir marta tasdiqlanishi kerak: %v", err)
	}
	_, u, _, err := e.auth.PollLogin(ctx, code)
	if err != nil || u.TelegramChatID != "555" {
		t.Fatalf("sessiya birinchi tasdiqlagan akkauntniki bo'lishi kerak: %+v %v", u, err)
	}
	if _, _, _, err := e.auth.PollLogin(ctx, "noma'lum"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("noma'lum kod rad etilishi kerak")
	}
	var ve *domain.ValidationError
	if err := e.auth.ConfirmLogin(ctx, code, domain.TelegramProfile{}); !errors.As(err, &ve) {
		t.Fatal("ID'siz profil rad etilishi kerak")
	}
}

func TestLogoutAll(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	prof := domain.TelegramProfile{ID: "555"}
	t1, _, _, _ := e.auth.PollLogin(ctx, startLogin(t, e, prof))
	t2, _, _, _ := e.auth.PollLogin(ctx, startLogin(t, e, prof))
	other, _, _, _ := e.auth.PollLogin(ctx, startLogin(t, e, domain.TelegramProfile{ID: "666"}))

	if err := e.auth.LogoutAll(ctx, "555"); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{t1, t2} {
		if _, err := e.auth.Authenticate(ctx, tok); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatal("barcha sessiyalar tugashi kerak")
		}
	}
	if _, err := e.auth.Authenticate(ctx, other); err != nil {
		t.Fatal("boshqa foydalanuvchi sessiyasi saqlanishi kerak")
	}
}

func TestWatchOwnership(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	a, b := e.user("A"), e.user("B")
	w := mustCreate(t, e, a, "2900000", "2900700", "2026-12-05")

	if err := e.ws.Delete(ctx, b.ID, w.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("B A'ning kuzatuvini o'chira olmasligi kerak: %v", err)
	}
	if _, err := e.ws.Toggle(ctx, b.ID, w.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("B A'ning kuzatuvini o'zgartira olmasligi kerak: %v", err)
	}
	if list, _ := e.ws.List(ctx, b.ID); len(list) != 0 {
		t.Fatal("B faqat o'z kuzatuvlarini ko'rishi kerak")
	}
	if err := e.ws.Delete(ctx, a.ID, w.ID); err != nil {
		t.Fatal(err)
	}
}

func TestWatchLimitPerUser(t *testing.T) {
	e := newEnv(2, 0)
	ctx := context.Background()
	u := e.user("A")
	w1 := mustCreate(t, e, u, "2900000", "2900700", "2026-12-01")
	mustCreate(t, e, u, "2900000", "2900700", "2026-12-02")

	var ve *domain.ValidationError
	if _, err := e.ws.Create(ctx, u.ID, usecase.CreateWatchInput{Route: domain.Route{From: "2900000", To: "2900700", Date: "2026-12-03"}}); !errors.As(err, &ve) {
		t.Fatalf("limit oshdi: %v", err)
	}
	// Bittasini to'xtatib, yangisini qo'shish mumkin; lekin to'xtatilganini qayta yoqib limitdan oshib bo'lmaydi.
	_, _ = e.ws.Toggle(ctx, u.ID, w1.ID)
	mustCreate(t, e, u, "2900000", "2900700", "2026-12-03")
	if _, err := e.ws.Toggle(ctx, u.ID, w1.ID); !errors.As(err, &ve) {
		t.Fatalf("qayta yoqishda ham limit tekshirilishi kerak: %v", err)
	}
}

func TestTestMessage(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	u := e.user("555")
	if err := e.tg.SendTest(ctx, u.ID); err != nil || e.notif.count("555") != 1 {
		t.Fatalf("test xabar yuborilmadi: %v", err)
	}
}

func TestSearchThrottlePerUser(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	r := domain.Route{From: "1", To: "2", Date: "2026-12-05"}

	if _, err := e.searchUC.Search(ctx, 1, r); err != nil {
		t.Fatal(err)
	}
	var te *usecase.ThrottledError
	if _, err := e.searchUC.Search(ctx, 1, r); !errors.As(err, &te) {
		t.Fatalf("throttle kutilgan: %v", err)
	}
	if _, err := e.searchUC.Search(ctx, 2, r); err != nil {
		t.Fatalf("boshqa foydalanuvchi cheklanmasligi kerak: %v", err)
	}
	e.clock.add(11 * time.Second)
	if _, err := e.searchUC.Search(ctx, 1, r); err != nil {
		t.Fatal(err)
	}
}

func TestPollingUpdateAdminOnly(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	user := e.users.add(domain.User{TelegramChatID: "100"})
	admin := e.users.add(domain.User{TelegramChatID: "900", Name: "Admin"})
	rules := []domain.PollingRule{{MaxDaysLeft: 3, Interval: 30 * time.Second}, {MaxDaysLeft: 0, Interval: 6 * time.Hour}}

	if _, err := e.polling.Update(ctx, user, rules); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("oddiy foydalanuvchi jadvalni o'zgartira olmasligi kerak: %v", err)
	}
	if _, err := e.polling.Stats(ctx, user); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("statistika ham faqat admin uchun")
	}
	p, err := e.polling.Update(ctx, admin, rules)
	if err != nil {
		t.Fatal(err)
	}
	if p.UpdatedBy != "Admin" || p.UpdatedAt.IsZero() {
		t.Fatalf("kim/qachon yozilmadi: %+v", p)
	}
	var ve *domain.ValidationError
	if _, err := e.polling.Update(ctx, admin, []domain.PollingRule{{MaxDaysLeft: 0, Interval: 5 * time.Second}}); !errors.As(err, &ve) {
		t.Fatal("10 soniyadan qisqa oraliq rad etilishi kerak")
	}
	if got, _ := e.polling.Policy(ctx); got.Rules[0].Interval != 30*time.Second {
		t.Fatal("noto'g'ri urinish saqlangan jadvalni buzmasligi kerak")
	}
}

func TestPollingStats(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	a, b := e.user("A"), e.user("B")
	mustCreate(t, e, a, "2900000", "2900700", "2026-11-24")
	mustCreate(t, e, b, "2900000", "2900700", "2026-11-24") // xuddi shu yo'nalish
	mustCreate(t, e, a, "2900000", "2900800", "2027-02-01")

	stats, err := e.polling.Stats(ctx, e.users.add(domain.User{TelegramChatID: "900"}))
	if err != nil {
		t.Fatal(err)
	}
	first, last := stats[0], stats[len(stats)-1]
	if first.Routes != 1 || first.Watches != 2 || first.RequestsPerHour != 360 {
		t.Fatalf("10 soniyalik bosqich: %+v", first)
	}
	if !last.Rule.IsDefault() || last.Routes != 1 || last.RequestsPerHour != 1.0/12 {
		t.Fatalf("qolganlari: %+v", last)
	}
}
