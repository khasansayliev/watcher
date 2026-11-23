package usecase

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/khasansayliev/railbot/internal/domain"
)

type MonitorConfig struct {
	Interval   time.Duration // asosiy tekshiruv oralig'i
	MaxBackoff time.Duration // xatolardan keyingi eng uzun pauza
	GroupPause time.Duration // turli yo'nalishlar so'rovlari orasidagi pauza
}

// Monitor faol kuzatuvlarni davriy tekshiradi va mos joy chiqsa xabar beradi.
type Monitor struct {
	repo     WatchRepository
	searcher TrainSearcher
	notifier Notifier
	clock    Clock
	cfg      MonitorConfig
	log      *slog.Logger
	trigger  chan string

	running sync.Mutex // bir vaqtda bitta sikl

	mu          sync.Mutex
	backoff     time.Duration
	pausedUntil time.Time
}

func NewMonitor(repo WatchRepository, s TrainSearcher, n Notifier, c Clock, cfg MonitorConfig, log *slog.Logger) *Monitor {
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = 30 * time.Minute
	}
	return &Monitor{repo: repo, searcher: s, notifier: n, clock: c, cfg: cfg, log: log, trigger: make(chan string, 16)}
}

func (m *Monitor) Interval() time.Duration { return m.cfg.Interval }

// PausedUntil — xatolar sababli tekshiruvlar qachongacha to'xtatilgan.
func (m *Monitor) PausedUntil() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pausedUntil
}

// RequestCheck — bitta kuzatuvni navbatdan tashqari tekshirish so'rovi (bloklamaydi).
func (m *Monitor) RequestCheck(id string) {
	select {
	case m.trigger <- id:
	default:
	}
}

func (m *Monitor) Run(ctx context.Context) {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			m.RunCycle(ctx, "")
			timer.Reset(jitter(m.cfg.Interval))
		case id := <-m.trigger:
			m.RunCycle(ctx, id)
		}
	}
}

// RunCycle bitta tekshiruv sikli. onlyID bo'sh bo'lmasa — faqat o'sha kuzatuv.
func (m *Monitor) RunCycle(ctx context.Context, onlyID string) {
	m.running.Lock()
	defer m.running.Unlock()

	if until := m.PausedUntil(); m.clock.Now().Before(until) {
		m.log.Info("tekshiruv pauzada", "until", until.Format("15:04:05"))
		return
	}

	watches, err := m.repo.List(ctx)
	if err != nil {
		m.log.Error("kuzatuvlarni o'qib bo'lmadi", "err", err)
		return
	}

	today := m.clock.Today()
	groups := map[string][]domain.Watch{}
	var order []domain.Route
	for _, w := range watches {
		if (onlyID != "" && w.ID != onlyID) || !w.Active {
			continue
		}
		if w.IsExpired(today) {
			_, _ = m.repo.Update(ctx, w.ID, func(x *domain.Watch) { x.Expire() })
			continue
		}
		// Bir xil yo'nalish+sana uchun bitta so'rov.
		k := w.Route.Key()
		if _, ok := groups[k]; !ok {
			order = append(order, w.Route)
		}
		groups[k] = append(groups[k], w)
	}

	for i, route := range order {
		if i > 0 && !sleepCtx(ctx, m.cfg.GroupPause) {
			return
		}
		trains, err := m.searcher.Search(ctx, route)
		now := m.clock.Now()
		if err != nil {
			d := m.fail(err)
			m.log.Warn("qidiruv xatosi", "route", route.Key(), "err", err, "pause", d.Round(time.Second))
			for _, w := range groups[route.Key()] {
				_, _ = m.repo.Update(ctx, w.ID, func(x *domain.Watch) { x.RecordFailure(now, err) })
			}
			return // qolganlari keyingi siklda — saytni qiynamaymiz
		}
		m.succeed()
		for _, w := range groups[route.Key()] {
			m.evaluate(ctx, w, trains, now)
		}
	}
}

func (m *Monitor) evaluate(ctx context.Context, w domain.Watch, trains []domain.Train, now time.Time) {
	matches, notify, sig := w.Evaluate(trains)
	var nerr error
	if notify {
		nerr = m.notifier.NotifyAvailability(ctx, w.ChatID, w.Route, matches)
		if nerr != nil {
			m.log.Warn("xabar yuborilmadi", "watch", w.ID, "err", nerr)
		}
	}
	_, _ = m.repo.Update(ctx, w.ID, func(x *domain.Watch) {
		x.RecordResult(now, len(trains), matches, sig, notify, nerr)
	})
}

func (m *Monitor) fail(err error) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.backoff == 0 {
		m.backoff = m.cfg.Interval
	} else {
		m.backoff *= 2
	}
	if m.backoff > m.cfg.MaxBackoff {
		m.backoff = m.cfg.MaxBackoff
	}
	d := m.backoff
	var ue *UpstreamError
	if errors.As(err, &ue) && ue.RetryAfter > d {
		d = ue.RetryAfter
	}
	m.pausedUntil = m.clock.Now().Add(d)
	return d
}

func (m *Monitor) succeed() {
	m.mu.Lock()
	m.backoff = 0
	m.pausedUntil = time.Time{}
	m.mu.Unlock()
}

// jitter: ±10% — so'rovlar bir daqiqaga yopishib qolmasin.
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	return d + time.Duration(rand.Int64N(int64(d)/5+1)) - d/10
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	d += time.Duration(rand.Int64N(int64(d)))
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
