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

func TestAuthLoginAndSession(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	e.idp.ident = domain.Identity{Subject: "g-1", Email: "ali@example.com", EmailVerified: true, Name: "Ali"}

	token, u, _, err := e.auth.Login(ctx, "code")
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 40 {
		t.Fatalf("token juda qisqa: %q", token)
	}
	if _, ok := e.sessions.m[token]; ok {
		t.Fatal("bazada token emas, uning xeshi saqlanishi kerak")
	}
	got, err := e.auth.Authenticate(ctx, token)
	if err != nil || got.ID != u.ID {
		t.Fatalf("sessiya ishlamadi: %v", err)
	}

	// Qayta kirish — yangi user yaratilmaydi.
	_, u2, _, _ := e.auth.Login(ctx, "code")
	if u2.ID != u.ID {
		t.Fatal("bir Google akkaunt — bitta foydalanuvchi bo'lishi kerak")
	}

	_ = e.auth.Logout(ctx, token)
	if _, err := e.auth.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("chiqqandan keyin sessiya yaroqsiz bo'lishi kerak")
	}

	token, _, _, _ = e.auth.Login(ctx, "code")
	e.clock.add(25 * time.Hour)
	if _, err := e.auth.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("muddati o'tgan sessiya rad etilishi kerak")
	}
}

func TestAuthRejectsUnverifiedEmail(t *testing.T) {
	e := newEnv(10, 0)
	e.idp.ident = domain.Identity{Subject: "g-1", Email: "x@example.com", EmailVerified: false}
	var ve *domain.ValidationError
	if _, _, _, err := e.auth.Login(context.Background(), "code"); !errors.As(err, &ve) {
		t.Fatalf("ValidationError kutilgan: %v", err)
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

func TestTelegramLinkFlow(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	u := e.users.add(domain.User{Email: "ali@example.com"})

	var ve *domain.ValidationError
	if _, err := e.ws.Create(ctx, u.ID, usecase.CreateWatchInput{Route: domain.Route{From: "1", To: "2", Date: "2026-12-05"}}); !errors.As(err, &ve) {
		t.Fatal("Telegram'siz kuzatuv yaratib bo'lmasligi kerak")
	}

	link, err := e.tg.StartLink(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	code := link[strings.Index(link, "start=")+6:]
	if err := e.tg.LinkChat(ctx, code, "555"); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.users.Get(ctx, u.ID); got.TelegramChatID != "555" {
		t.Fatal("chat bog'lanmadi")
	}
	if err := e.tg.LinkChat(ctx, code, "666"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("kod faqat bir marta ishlashi kerak")
	}

	link, _ = e.tg.StartLink(ctx, u.ID)
	e.clock.add(16 * time.Minute)
	if err := e.tg.LinkChat(ctx, link[strings.Index(link, "start=")+6:], "777"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("eskirgan kod rad etilishi kerak")
	}
}

func TestTelegramChatMovesBetweenAccounts(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	a := e.user("555")
	b := e.users.add(domain.User{Email: "b@example.com"})
	link, _ := e.tg.StartLink(ctx, b.ID)
	_ = e.tg.LinkChat(ctx, link[strings.Index(link, "start=")+6:], "555")

	if got, _ := e.users.Get(ctx, a.ID); got.TelegramChatID != "" {
		t.Fatal("bitta chat ikki akkauntga bog'lanib qolmasligi kerak")
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
	user := e.users.add(domain.User{Email: "ali@example.com"})
	admin := e.users.add(domain.User{Email: "Admin@Example.com"})
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
	if p.UpdatedBy != "Admin@Example.com" || p.UpdatedAt.IsZero() {
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

	stats, err := e.polling.Stats(ctx, e.users.add(domain.User{Email: "admin@example.com"}))
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
