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

func params(from, to, date string) usecase.CreateWatchInput {
	return usecase.CreateWatchInput{Route: domain.Route{From: from, To: to, Date: date}}
}

func TestEditWatchChangesRouteAndFilterAndResetsProgress(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	u := e.user("A")
	w := mustCreate(t, e, u, "2900000", "2900700", "2026-12-05")
	e.monitor.RunCycle(ctx, 0)
	before, _ := e.watches.Get(ctx, w.ID)
	if before.LastSig == "" || before.LastCheck.IsZero() {
		t.Fatal("tayyorgarlik: kuzatuv tekshirilgan bo'lishi kerak")
	}
	sent := e.notif.count("A")

	in := params("2900000", "2900800", "2026-12-10")
	in.Filter = domain.Filter{MaxPrice: 500000, MinSeats: 2, CarType: " сидяч ", Window: domain.TimeWindow{From: "08:00", To: "18:00"}}
	got, err := e.ws.Edit(ctx, u.ID, w.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != w.ID || got.Route.To != "2900800" || got.Route.Date != "2026-12-10" ||
		got.Filter.MaxPrice != 500000 || got.Filter.MinSeats != 2 || got.Filter.CarType != "сидяч" ||
		got.Filter.Window.From != "08:00" || got.Filter.Window.To != "18:00" {
		t.Fatalf("o'zgarishlar qo'llanmadi: %+v", got)
	}
	if !got.LastCheck.IsZero() || got.LastSig != "" || got.LastMatches != 0 || !strings.Contains(got.LastStatus, "O'zgartirildi") {
		t.Fatalf("tekshiruv holati boshidan boshlanishi kerak: %+v", got)
	}
	if got.CreatedAt != before.CreatedAt || !got.Active || got.UserID != u.ID {
		t.Fatalf("egasi/yaratilgan vaqt/faollik saqlanishi kerak: %+v", got)
	}

	// Darhol tekshiriladi va yangidan xabar beradi (imzo tozalangan).
	e.monitor.RunCycle(ctx, w.ID)
	if after, _ := e.watches.Get(ctx, w.ID); after.LastCheck.IsZero() {
		t.Fatal("tahrirdan keyin tekshirilishi kerak")
	}
	if e.notif.count("A") != sent+1 {
		t.Fatal("yangi shart bo'yicha xabar yuborilishi kerak")
	}
}

func TestEditWatchRules(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	admin := e.admin900()
	a, b := e.user("A"), e.user("B")
	w := mustCreate(t, e, a, "2900000", "2900700", "2026-12-05")
	ok := params("2900000", "2900800", "2026-12-06")

	// Boshqa foydalanuvchi o'zgartira olmaydi.
	if _, err := e.ws.Edit(ctx, b.ID, w.ID, ok); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("B A'ning kuzatuvini tahrirlay olmasligi kerak: %v", err)
	}
	if _, err := e.ws.Get(ctx, b.ID, w.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("B A'ning kuzatuvini ko'ra olmasligi kerak")
	}

	// Yaratish bilan bir xil tekshiruvlar.
	var ve *domain.ValidationError
	bad := []usecase.CreateWatchInput{
		params("2900000", "2900000", "2026-12-06"), // bir xil stansiya
		params("2900000", "2900800", "2026-11-01"), // o'tgan sana
		params("2900000", "2900800", "noto'g'ri"),
	}
	badWindow := ok
	badWindow.Filter.Window = domain.TimeWindow{From: "18:00", To: "08:00"}
	bad = append(bad, badWindow)
	for i, in := range bad {
		if _, err := e.ws.Edit(ctx, a.ID, w.ID, in); !errors.As(err, &ve) {
			t.Errorf("#%d: ValidationError kutilgan: %v", i, err)
		}
	}
	if got, _ := e.watches.Get(ctx, w.ID); got.Route.To != "2900700" {
		t.Fatal("rad etilgan tahrir kuzatuvni o'zgartirmasligi kerak")
	}

	// Ruxsati olinsa, tahrirlash orqali ham aylanib o'tib bo'lmaydi.
	_ = e.admin.SetCanCreate(ctx, admin, a.ID, false)
	if _, err := e.ws.Edit(ctx, a.ID, w.ID, ok); !errors.As(err, &ve) || !strings.Contains(ve.Msg, "ruxsat") {
		t.Fatalf("ruxsatsiz tahrirlash rad etilishi kerak: %v", err)
	}
}

func TestEditKeepsAdminControlsAndExpiredReactivation(t *testing.T) {
	e := newEnv(1, 0)
	ctx := context.Background()
	admin := e.admin900()
	u := e.user("A")
	w := mustCreate(t, e, u, "2900000", "2900700", "2026-12-05")
	_ = e.admin.SetWatchInterval(ctx, admin, w.ID, 45*time.Second)
	_ = e.admin.SetWatchDisabled(ctx, admin, w.ID, true)

	got, err := e.ws.Edit(ctx, u.ID, w.ID, params("2900000", "2900800", "2026-12-07"))
	if err != nil {
		t.Fatal(err)
	}
	if !got.AdminDisabled || got.CheckEvery != 45*time.Second {
		t.Fatalf("admin belgilari o'zgarmasligi kerak: %+v", got)
	}
	calls := e.search.calls
	e.monitor.RunCycle(ctx, w.ID)
	if e.search.calls != calls {
		t.Fatal("tahrirlash admin to'xtatgan kuzatuvni qayta yoqmasligi kerak")
	}
	_ = e.admin.SetWatchDisabled(ctx, admin, w.ID, false)

	// Sanasi o'tib to'xtagan kuzatuv yangi sana bilan qayta yoqiladi.
	e.clock.add(40 * 24 * time.Hour) // 2026-01-02 → sana o'tdi
	e.monitor.RunCycle(ctx, 0)
	if x, _ := e.watches.Get(ctx, w.ID); x.Active {
		t.Fatal("tayyorgarlik: sana o'tgach to'xtashi kerak")
	}
	re, err := e.ws.Edit(ctx, u.ID, w.ID, params("2900000", "2900800", "2027-01-20"))
	if err != nil || !re.Active {
		t.Fatalf("yangi sana bilan qayta yoqilishi kerak: %+v %v", re, err)
	}

	// Limitga to'lgan bo'lsa (1 ta faol), to'xtagan kuzatuvni tahrirlab qayta yoqib bo'lmaydi.
	e.clock.add(400 * 24 * time.Hour)
	e.monitor.RunCycle(ctx, 0)
	mustCreate(t, e, u, "2900000", "2900700", "2028-12-01")
	var ve *domain.ValidationError
	if _, err := e.ws.Edit(ctx, u.ID, w.ID, params("2900000", "2900800", "2028-12-05")); !errors.As(err, &ve) {
		t.Fatalf("limit tekshirilishi kerak: %v", err)
	}
}

// Tahrir so'rov yuborilgan paytda bo'lsa, eski yo'nalish natijasi yangi kuzatuvga yozilmaydi.
func TestStaleResultDoesNotOverwriteEdit(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	u := e.user("A")
	w := mustCreate(t, e, u, "2900000", "2900700", "2026-12-05")
	e.search.onSearch = func() {
		e.search.onSearch = nil
		if _, err := e.ws.Edit(ctx, u.ID, w.ID, params("2900000", "2900800", "2026-12-06")); err != nil {
			t.Error(err)
		}
	}
	e.monitor.RunCycle(ctx, 0)

	got, _ := e.watches.Get(ctx, w.ID)
	if got.Route.To != "2900800" {
		t.Fatalf("tahrir saqlanishi kerak: %+v", got.Route)
	}
	if !got.LastCheck.IsZero() || got.LastSig != "" || !strings.Contains(got.LastStatus, "O'zgartirildi") {
		t.Fatalf("eski natija yozilmasligi kerak (yangi shart darhol qayta tekshiriladi): %+v", got)
	}
	if e.notif.count("A") != 0 {
		t.Fatal("eski yo'nalish bo'yicha xabar yuborilmasligi kerak")
	}
}

// Bildirishnoma yuborilmaydigan holatda ham (mos poyezd yo'q) eskirgan natija logga yozilmaydi.
func TestStaleResultWithoutNotificationIsNotLogged(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	u := e.user("A")
	w := mustCreate(t, e, u, "2900000", "2900700", "2026-12-05")
	e.search.trains = nil // mos poyezd yo'q — xabar ketmaydi
	e.search.onSearch = func() {
		e.search.onSearch = nil
		_, _ = e.ws.Edit(ctx, u.ID, w.ID, params("2900000", "2900800", "2026-12-06"))
	}
	e.monitor.RunCycle(ctx, 0)
	if n := len(e.logs.forWatch(w.ID)); n != 0 {
		t.Fatalf("eskirgan so'rov logi yozilmasligi kerak: %d", n)
	}
	if got, _ := e.watches.Get(ctx, w.ID); !got.LastCheck.IsZero() {
		t.Fatal("eskirgan natija holatni buzmasligi kerak")
	}
}
