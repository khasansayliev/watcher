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

func mustCreate(t *testing.T, e *env, u domain.User, from, to, date string) domain.Watch {
	t.Helper()
	w, err := e.ws.Create(context.Background(), u.ID, usecase.CreateWatchInput{Route: domain.Route{From: from, To: to, Date: date}})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestMonitorOneRequestPerRouteAcrossUsers(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	a, b := e.user("A"), e.user("B")
	mustCreate(t, e, a, "2900000", "2900700", "2026-12-05")
	mustCreate(t, e, b, "2900000", "2900700", "2026-12-05") // boshqa foydalanuvchi, xuddi shu yo'nalish

	e.monitor.RunCycle(ctx, 0)
	if e.search.calls != 1 {
		t.Fatalf("bir xil yo'nalish uchun 1 so'rov kutilgan, %d", e.search.calls)
	}
	if e.notif.count(a.ChatID()) != 1 || e.notif.count(b.ChatID()) != 1 {
		t.Fatalf("har bir foydalanuvchi o'z chatiga xabar olishi kerak: %+v", e.notif.sent)
	}

	e.monitor.RunCycle(ctx, 0)
	if e.notif.count(a.ChatID()) != 1 {
		t.Fatal("o'zgarish yo'q — takror xabar bo'lmasligi kerak")
	}
}

func TestMonitorRespectsSearchCapAndRotates(t *testing.T) {
	e := newEnv(10, 2) // siklda ko'pi bilan 2 so'rov
	ctx := context.Background()
	u := e.user("A")
	for _, d := range []string{"2026-12-01", "2026-12-02", "2026-12-03"} {
		mustCreate(t, e, u, "2900000", "2900700", d)
	}

	e.monitor.RunCycle(ctx, 0)
	if e.search.calls != 2 {
		t.Fatalf("chegara 2, %d so'rov ketdi", e.search.calls)
	}
	e.clock.add(time.Minute)
	e.monitor.RunCycle(ctx, 0)

	ws, _ := e.watches.ListByUser(ctx, u.ID)
	for _, w := range ws {
		if w.LastCheck.IsZero() {
			t.Fatalf("2 sikldan keyin hamma yo'nalish tekshirilgan bo'lishi kerak: %s", w.Route.Date)
		}
	}
}

<<<<<<< Updated upstream
func TestMonitorUserBlockedBot(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	u := e.user("A")
	w := mustCreate(t, e, u, "2900000", "2900700", "2026-12-05")
	e.notif.blocked = map[string]bool{u.ChatID(): true}

	e.monitor.RunCycle(ctx, 0)
	got, _ := e.watches.Get(ctx, w.ID)
	if got.LastSig != "" || !strings.Contains(got.LastStatus, "Telegram xato") {
		t.Fatalf("xabar yetmasa imzo saqlanmasligi va xato ko'rinishi kerak: %+v", got)
	}
	e.notif.blocked = nil // foydalanuvchi botni qayta yoqdi
	e.clock.add(2 * time.Hour)
	e.monitor.RunCycle(ctx, 0)
	if e.notif.count(u.ChatID()) != 1 {
		t.Fatal("blokdan chiqqach xabar kelishi kerak")
	}
}

=======
>>>>>>> Stashed changes
func TestMonitorBackoffOnUpstreamError(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	w := mustCreate(t, e, e.user("A"), "2900000", "2900700", "2026-12-05")

	e.search.err = &usecase.UpstreamError{Status: 429, RetryAfter: 20 * time.Minute}
	e.monitor.RunCycle(ctx, 0)
	if got := e.monitor.PausedUntil().Sub(e.clock.Now()); got != 20*time.Minute {
		t.Fatalf("Retry-After'ga amal qilinmadi: %s", got)
	}
	if got, _ := e.watches.Get(ctx, w.ID); got.LastStatus == "" {
		t.Fatal("xato holati yozilmadi")
	}

	calls := e.search.calls
	e.monitor.RunCycle(ctx, 0)
	if e.search.calls != calls {
		t.Fatal("pauza vaqtida so'rov yuborildi")
	}
	var pe *usecase.PausedError
	if _, err := e.searchUC.Search(ctx, 1, domain.Route{From: "1", To: "2", Date: "2026-12-05"}); !errors.As(err, &pe) {
		t.Fatalf("qo'lda qidiruv ham pauzada bo'lishi kerak: %v", err)
	}

	e.clock.add(61 * time.Minute) // pauza tugadi va 1 soatlik oraliq ham o'tdi
	e.search.err = nil
	e.monitor.RunCycle(ctx, 0)
	if !e.monitor.PausedUntil().IsZero() {
		t.Fatal("muvaffaqiyatdan keyin pauza tozalanishi kerak")
	}
}

func TestMonitorExpiresPastWatches(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	w := mustCreate(t, e, e.user("A"), "2900000", "2900700", "2026-11-24")

	e.clock.add(72 * time.Hour)
	e.monitor.RunCycle(ctx, 0)
	got, _ := e.watches.Get(ctx, w.ID)
	if got.Active || e.search.calls != 0 {
		t.Fatalf("o'tgan sana to'xtatilishi va so'ralmasligi kerak: active=%v calls=%d", got.Active, e.search.calls)
	}
}

func TestMonitorChecksOnlyDueRoutes(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	u := e.user("A")
	mustCreate(t, e, u, "2900000", "2900700", "2026-11-24") // ertaga: 10 s
	mustCreate(t, e, u, "2900000", "2900800", "2026-11-28") // 5 kun: 5 daqiqa
	mustCreate(t, e, u, "2900000", "2900930", "2027-01-10") // 48 kun: 12 soat

	e.monitor.RunCycle(ctx, 0)
	if e.search.calls != 3 {
		t.Fatalf("birinchi siklda hammasi tekshirilishi kerak, %d", e.search.calls)
	}
	step := func(d time.Duration, wantCalls int) {
		t.Helper()
		before := e.search.calls
		e.clock.add(d)
		e.monitor.RunCycle(ctx, 0)
		if got := e.search.calls - before; got != wantCalls {
			t.Fatalf("+%s: %d so'rov, kutilgan %d", d, got, wantCalls)
		}
	}
	step(11*time.Second, 1) // faqat ertangi
	step(5*time.Minute, 2)  // ertangi + 5 kunlik; 48 kunlik hali emas
	step(12*time.Hour, 3)   // hammasining vaqti keldi
}

func TestMonitorPolicyChangeAppliesWithoutRestart(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	mustCreate(t, e, e.user("A"), "2900000", "2900700", "2027-01-10") // 12 soat
	e.monitor.RunCycle(ctx, 0)

<<<<<<< Updated upstream
	admin := e.admin()
=======
	admin := e.users.add(domain.User{TelegramChatID: "900"})
>>>>>>> Stashed changes
	if _, err := e.polling.Update(ctx, admin, []domain.PollingRule{{MaxDaysLeft: 0, Interval: time.Minute}}); err != nil {
		t.Fatal(err)
	}
	before := e.search.calls
	e.clock.add(61 * time.Second)
	e.monitor.RunCycle(ctx, 0)
	if e.search.calls != before+1 {
		t.Fatal("yangi jadval keyingi siklda kuchga kirishi kerak")
	}
}

func TestMonitorShortIntervalStillBacksOffOneMinute(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	mustCreate(t, e, e.user("A"), "2900000", "2900700", "2026-11-24") // ertaga — har 10 soniya

	e.search.err = &usecase.UpstreamError{Status: 503}
	e.monitor.RunCycle(ctx, 0)
	if got := e.monitor.PausedUntil().Sub(e.clock.Now()); got != time.Minute {
		t.Fatalf("10s oraliqda ham birinchi pauza 1 daqiqa bo'lishi kerak, %s", got)
	}
	e.clock.add(61 * time.Second)
	e.monitor.RunCycle(ctx, 0)
	if got := e.monitor.PausedUntil().Sub(e.clock.Now()); got != 2*time.Minute {
		t.Fatalf("ikkinchi xatoda pauza ikki baravar oshishi kerak, %s", got)
	}
}

func TestMonitorDueSlackForTenSecondTier(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	mustCreate(t, e, e.user("A"), "2900000", "2900700", "2026-11-24") // 10 s
	e.monitor.RunCycle(ctx, 0)
	e.clock.add(9 * time.Second) // jitter tufayli monitor biroz erta uyg'ondi
	e.monitor.RunCycle(ctx, 0)
	if e.search.calls != 2 {
		t.Fatalf("9 soniyada ham tekshirilishi kerak (2 s yo'l qo'yish), so'rovlar: %d", e.search.calls)
	}
	e.clock.add(5 * time.Second)
	e.monitor.RunCycle(ctx, 0)
	if e.search.calls != 2 {
		t.Fatal("5 soniyada qayta tekshirilmasligi kerak")
	}
}
