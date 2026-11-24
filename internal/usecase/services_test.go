package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

func TestAuthLoginAndSession(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	e.verifier.ident = domain.Identity{TelegramID: 777, FirstName: "Ali", Username: "ali", AuthDate: e.clock.Now().Add(-time.Minute)}

	token, u, _, err := e.auth.Login(ctx, map[string]string{"id": "777"})
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 40 || u.TelegramID != 777 || u.ChatID() != "777" {
		t.Fatalf("token/user: %q %+v", token, u)
	}
	if _, ok := e.sessions.m[token]; ok {
		t.Fatal("bazada token emas, uning xeshi saqlanishi kerak")
	}
	if got, err := e.auth.Authenticate(ctx, token); err != nil || got.ID != u.ID {
		t.Fatalf("sessiya ishlamadi: %v", err)
	}

	// Qayta kirish — yangi user yaratilmaydi, profil yangilanadi.
	e.verifier.ident.Username = "ali_new"
	_, u2, _, _ := e.auth.Login(ctx, nil)
	if u2.ID != u.ID || u2.Username != "ali_new" {
		t.Fatalf("bitta Telegram akkaunt — bitta foydalanuvchi: %+v", u2)
	}

	_ = e.auth.Logout(ctx, token)
	if _, err := e.auth.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("chiqqandan keyin sessiya yaroqsiz bo'lishi kerak")
	}
	token, _, _, _ = e.auth.Login(ctx, nil)
	e.clock.add(25 * time.Hour)
	if _, err := e.auth.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("muddati o'tgan sessiya rad etilishi kerak")
	}
}

func TestAuthRejectsBadSignatureAndStaleData(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()

	e.verifier.valid = false
	if _, _, _, err := e.auth.Login(ctx, nil); !errors.Is(err, usecase.ErrBadSignature) {
		t.Fatalf("soxta imzo rad etilishi kerak: %v", err)
	}

	e.verifier.valid = true
	e.verifier.ident = domain.Identity{TelegramID: 777, AuthDate: e.clock.Now().Add(-2 * time.Hour)}
	var ve *domain.ValidationError
	if _, _, _, err := e.auth.Login(ctx, nil); !errors.As(err, &ve) {
		t.Fatalf("eskirgan (qayta ishlatilgan) ma'lumot rad etilishi kerak: %v", err)
	}
	if len(e.users.m) != 0 || len(e.sessions.m) != 0 {
		t.Fatal("rad etilgan urinish foydalanuvchi yoki sessiya yaratmasligi kerak")
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

func TestTelegramSendTest(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	u := e.user("A")
	if err := e.tg.SendTest(ctx, u.ID); err != nil || e.notif.count(u.ChatID()) != 1 {
		t.Fatalf("test xabar o'z chatiga borishi kerak: %v", err)
	}
	e.notif.blocked = map[string]bool{u.ChatID(): true}
	if err := e.tg.SendTest(ctx, u.ID); !errors.Is(err, usecase.ErrBotBlocked) {
		t.Fatalf("bloklangan bot uchun ErrBotBlocked kutilgan: %v", err)
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
	user := e.user("ali")
	admin := e.admin()
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
	if p.UpdatedBy != "@admin" || p.UpdatedAt.IsZero() {
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

	stats, err := e.polling.Stats(ctx, e.admin())
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
