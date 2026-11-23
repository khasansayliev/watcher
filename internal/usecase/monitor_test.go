package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/khasansayliev/railbot/internal/domain"
	"github.com/khasansayliev/railbot/internal/usecase"
)

// ---- fake portlar: usecase tarmoqsiz, to'liq izolyatsiyada tekshiriladi ----

type memRepo struct {
	mu sync.Mutex
	ws []domain.Watch
	n  int
}

func (r *memRepo) Create(_ context.Context, w domain.Watch) (domain.Watch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n++
	w.ID = strconv.Itoa(r.n)
	r.ws = append(r.ws, w)
	return w, nil
}
func (r *memRepo) Get(_ context.Context, id string) (domain.Watch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, w := range r.ws {
		if w.ID == id {
			return w, nil
		}
	}
	return domain.Watch{}, domain.ErrNotFound
}
func (r *memRepo) List(context.Context) ([]domain.Watch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.Watch(nil), r.ws...), nil
}
func (r *memRepo) Update(_ context.Context, id string, fn func(*domain.Watch)) (domain.Watch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.ws {
		if r.ws[i].ID == id {
			fn(&r.ws[i])
			return r.ws[i], nil
		}
	}
	return domain.Watch{}, domain.ErrNotFound
}
func (r *memRepo) Delete(context.Context, string) error { return nil }

type fakeSearcher struct {
	calls  int
	trains []domain.Train
	err    error
}

func (f *fakeSearcher) Search(context.Context, domain.Route) ([]domain.Train, error) {
	f.calls++
	return f.trains, f.err
}

type fakeNotifier struct{ sent []string }

func (f *fakeNotifier) Enabled() bool { return true }
func (f *fakeNotifier) NotifyAvailability(_ context.Context, chat string, _ domain.Route, _ []domain.Match) error {
	f.sent = append(f.sent, chat)
	return nil
}
func (f *fakeNotifier) SendTest(context.Context, string) error { return nil }

type fixedClock struct{ t time.Time }

func (c *fixedClock) Now() time.Time { return c.t }
func (c *fixedClock) Today() string  { return c.t.Format(domain.DateLayout) }

var sample = []domain.Train{{Number: "764Ф", Cars: []domain.Car{{Type: "Сидячий", FreeSeats: 5, Price: 270000}}}}

func setup() (*memRepo, *fakeSearcher, *fakeNotifier, *fixedClock, *usecase.Monitor, *usecase.WatchService) {
	repo := &memRepo{}
	s := &fakeSearcher{trains: sample}
	n := &fakeNotifier{}
	clk := &fixedClock{t: time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := usecase.NewMonitor(repo, s, n, clk, usecase.MonitorConfig{Interval: 5 * time.Minute}, log)
	ws := usecase.NewWatchService(repo, n, clk, m)
	return repo, s, n, clk, m, ws
}

func create(t *testing.T, ws *usecase.WatchService, chat, date string) domain.Watch {
	t.Helper()
	w, err := ws.Create(context.Background(), usecase.CreateWatchInput{
		Route: domain.Route{From: "2900000", To: "2900700", Date: date}, ChatID: chat,
	})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestMonitorGroupsAndDeduplicates(t *testing.T) {
	_, s, n, _, m, ws := setup()
	ctx := context.Background()
	create(t, ws, "A", "2026-12-05")
	create(t, ws, "B", "2026-12-05") // xuddi shu yo'nalish

	m.RunCycle(ctx, "")
	if s.calls != 1 {
		t.Fatalf("bir xil yo'nalish uchun 1 so'rov kutilgan, %d", s.calls)
	}
	if len(n.sent) != 2 {
		t.Fatalf("2 ta xabar kutilgan, %v", n.sent)
	}

	m.RunCycle(ctx, "")
	if len(n.sent) != 2 {
		t.Fatalf("o'zgarish yo'q — yangi xabar bo'lmasligi kerak, %v", n.sent)
	}
}

func TestMonitorBackoffOnUpstreamError(t *testing.T) {
	repo, s, _, clk, m, ws := setup()
	ctx := context.Background()
	w := create(t, ws, "A", "2026-12-05")

	s.err = &usecase.UpstreamError{Status: 429, RetryAfter: 20 * time.Minute}
	m.RunCycle(ctx, "")
	if got := m.PausedUntil().Sub(clk.t); got != 20*time.Minute {
		t.Fatalf("Retry-After'ga amal qilinmadi: %s", got)
	}
	got, _ := repo.Get(ctx, w.ID)
	if got.LastStatus == "" || got.LastCheck.IsZero() {
		t.Fatal("xato holati yozilmadi")
	}

	calls := s.calls
	m.RunCycle(ctx, "") // pauza davomida so'rov ketmasligi kerak
	if s.calls != calls {
		t.Fatal("pauza vaqtida so'rov yuborildi")
	}

	clk.t = clk.t.Add(21 * time.Minute)
	s.err = nil
	m.RunCycle(ctx, "")
	if !m.PausedUntil().IsZero() {
		t.Fatal("muvaffaqiyatdan keyin pauza tozalanishi kerak")
	}
}

func TestMonitorExpiresPastWatches(t *testing.T) {
	repo, s, _, clk, m, ws := setup()
	ctx := context.Background()
	w := create(t, ws, "A", "2026-11-24")

	clk.t = clk.t.Add(72 * time.Hour)
	m.RunCycle(ctx, "")
	got, _ := repo.Get(ctx, w.ID)
	if got.Active || s.calls != 0 {
		t.Fatalf("o'tgan sana to'xtatilishi va so'ralmasligi kerak: active=%v calls=%d", got.Active, s.calls)
	}
}

func TestSearchServiceThrottle(t *testing.T) {
	_, s, _, clk, m, _ := setup()
	svc := usecase.NewSearchService(s, m, clk, 10*time.Second)
	r := domain.Route{From: "1", To: "2", Date: "2026-12-05"}
	ctx := context.Background()

	if _, err := svc.Search(ctx, r); err != nil {
		t.Fatal(err)
	}
	var te *usecase.ThrottledError
	if _, err := svc.Search(ctx, r); !errors.As(err, &te) {
		t.Fatalf("throttle kutilgan, %v", err)
	}
	clk.t = clk.t.Add(11 * time.Second)
	if _, err := svc.Search(ctx, r); err != nil {
		t.Fatal(err)
	}
}
