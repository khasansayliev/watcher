package usecase_test

import (
	"context"
	"errors"
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
	if e.notif.count("A") != 1 || e.notif.count("B") != 1 {
		t.Fatalf("har bir foydalanuvchi o'z chatiga xabar olishi kerak: %+v", e.notif.sent)
	}

	e.monitor.RunCycle(ctx, 0)
	if e.notif.count("A") != 1 {
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

func TestMonitorUserWithoutTelegram(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	u := e.user("A")
	w := mustCreate(t, e, u, "2900000", "2900700", "2026-12-05")
	_ = e.tg.Unlink(ctx, u.ID) // kuzatuv yaratilgandan keyin uzdi

	e.monitor.RunCycle(ctx, 0)
	got, _ := e.watches.Get(ctx, w.ID)
	if got.LastSig != "" || len(e.notif.sent) != 0 {
		t.Fatal("Telegram yo'q — xabar ketmasligi va imzo saqlanmasligi kerak")
	}
	_ = e.users.SetTelegramChat(ctx, u.ID, "A2")
	e.monitor.RunCycle(ctx, 0)
	if e.notif.count("A2") != 1 {
		t.Fatal("qayta ulangach xabar kelishi kerak")
	}
}

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

	e.clock.add(21 * time.Minute)
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
