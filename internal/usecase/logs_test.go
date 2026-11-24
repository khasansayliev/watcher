package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

func TestMonitorLogsEverySuccessfulRequest(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	u := e.user("A")
	w := mustCreate(t, e, u, "2900000", "2900700", "2026-12-05")
	other := mustCreate(t, e, e.user("B"), "2900000", "2900700", "2026-12-05") // xuddi shu yo'nalish

	e.monitor.RunCycle(ctx, w.ID) // qo'lda
	e.clock.add(15 * time.Second)
	e.monitor.RunCycle(ctx, 0) // avtomatik

	logs := e.logs.forWatch(w.ID)
	if len(logs) != 2 {
		t.Fatalf("2 ta so'rov — 2 ta log kutilgan: %d", len(logs))
	}
	if !logs[0].ReqTime.After(logs[1].ReqTime) {
		t.Fatal("yangisi birinchi bo'lishi kerak")
	}
	first, second := logs[1], logs[0]
	if !first.Manual || second.Manual {
		t.Fatalf("manual belgisi: %v / %v", first.Manual, second.Manual)
	}
	if !first.OK || first.TrainCount != 1 || first.MatchCount != 1 || !first.Notified || first.Error != "" {
		t.Fatalf("birinchi log: %+v", first)
	}
	if second.Notified {
		t.Fatal("o'zgarish yo'q — ikkinchi so'rovda xabar yuborilmaydi")
	}
	if len(first.Trains) != 1 || first.Trains[0].Number != "764Ф" || !first.Trains[0].Matched ||
		len(first.Trains[0].Cars) != 1 || first.Trains[0].Cars[0].Seats != 5 {
		t.Fatalf("natija snapshot'i: %+v", first.Trains)
	}
	// Qo'lda tekshiruv faqat shu kuzatuv uchun; ikkinchi sikl ikkalasi uchun.
	if got := len(e.logs.forWatch(other.ID)); got != 1 {
		t.Fatalf("boshqa kuzatuv uchun 1 ta log kutilgan: %d", got)
	}
}

func TestMonitorLogsFailedRequest(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	w := mustCreate(t, e, e.user("A"), "2900000", "2900700", "2026-12-05")
	e.search.err = &usecase.UpstreamError{Status: 504, Msg: "Gateway Time-out"}

	e.monitor.RunCycle(ctx, 0)

	logs := e.logs.forWatch(w.ID)
	if len(logs) != 1 || logs[0].OK || logs[0].Error == "" || logs[0].MatchCount != 0 {
		t.Fatalf("xato logi: %+v", logs)
	}
	if want := "504"; !contains(logs[0].Error, want) {
		t.Fatalf("xato matnida HTTP status bo'lishi kerak: %q", logs[0].Error)
	}
	// Pauza vaqtida so'rov yuborilmaydi — log ham yozilmaydi.
	e.monitor.RunCycle(ctx, 0)
	if got := len(e.logs.forWatch(w.ID)); got != 1 {
		t.Fatalf("pauzada log yozilmasligi kerak: %d", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestWatchLogsPaginationAndOwnership(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	a, b := e.user("A"), e.user("B")
	w := mustCreate(t, e, a, "2900000", "2900700", "2026-12-05")
	base := e.clock.Now()
	total := usecase.LogsPerPage*2 + 5 // 3 sahifa
	for i := 0; i < total; i++ {
		_ = e.logs.Add(ctx, domain.CheckLog{WatchID: w.ID, ReqTime: base.Add(time.Duration(i) * time.Second), OK: true, TrainCount: i})
	}

	p1, err := e.ws.Logs(ctx, a.ID, w.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if p1.Total != total || p1.Pages != 3 || p1.Page != 1 || len(p1.Logs) != usecase.LogsPerPage {
		t.Fatalf("1-sahifa: %+v", p1)
	}
	if p1.Logs[0].TrainCount != total-1 {
		t.Fatalf("eng yangi log birinchi bo'lishi kerak: %d", p1.Logs[0].TrainCount)
	}
	for i := 1; i < len(p1.Logs); i++ {
		if !p1.Logs[i-1].ReqTime.After(p1.Logs[i].ReqTime) {
			t.Fatal("req_time bo'yicha kamayish tartibi buzilgan")
		}
	}
	p3, _ := e.ws.Logs(ctx, a.ID, w.ID, 3)
	if len(p3.Logs) != 5 || p3.Logs[len(p3.Logs)-1].TrainCount != 0 {
		t.Fatalf("3-sahifa (eng eskilar): %+v", p3)
	}
	seen := map[int64]bool{}
	for _, p := range []int{1, 2, 3} {
		pg, _ := e.ws.Logs(ctx, a.ID, w.ID, p)
		for _, l := range pg.Logs {
			if seen[l.ID] {
				t.Fatal("yozuv ikki sahifada takrorlandi")
			}
			seen[l.ID] = true
		}
	}
	if len(seen) != total {
		t.Fatalf("hamma yozuv ko'rinishi kerak: %d/%d", len(seen), total)
	}

	// Chegaradan tashqari sahifa eng yaqin to'g'ri sahifaga keltiriladi.
	if far, _ := e.ws.Logs(ctx, a.ID, w.ID, 99); far.Page != 3 || len(far.Logs) != 5 {
		t.Fatalf("99-sahifa: %+v", far)
	}
	if zero, _ := e.ws.Logs(ctx, a.ID, w.ID, 0); zero.Page != 1 {
		t.Fatal("0-sahifa 1 ga tenglanadi")
	}
	// Boshqa foydalanuvchi ko'ra olmaydi.
	if _, err := e.ws.Logs(ctx, b.ID, w.ID, 1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("B A'ning loglarini ko'ra olmasligi kerak: %v", err)
	}
	// Loglari bo'lmagan kuzatuv: 1 sahifa, bo'sh.
	empty := mustCreate(t, e, a, "2900000", "2900800", "2026-12-06")
	if pg, _ := e.ws.Logs(ctx, a.ID, empty.ID, 1); pg.Pages != 1 || pg.Total != 0 || len(pg.Logs) != 0 {
		t.Fatalf("bo'sh: %+v", pg)
	}
}

func TestPruneLogs(t *testing.T) {
	e := newEnv(10, 0)
	ctx := context.Background()
	w := mustCreate(t, e, e.user("A"), "2900000", "2900700", "2026-12-05")
	now := e.clock.Now()
	_ = e.logs.Add(ctx, domain.CheckLog{WatchID: w.ID, ReqTime: now.Add(-20 * 24 * time.Hour)})
	_ = e.logs.Add(ctx, domain.CheckLog{WatchID: w.ID, ReqTime: now.Add(-2 * 24 * time.Hour)})

	n, err := e.ws.PruneLogs(ctx, 14*24*time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("prune: %d %v", n, err)
	}
	if len(e.logs.forWatch(w.ID)) != 1 {
		t.Fatal("yangi log saqlanishi kerak")
	}
}
