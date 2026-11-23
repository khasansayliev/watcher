package usecase

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sort"
	"sync"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
)

// minBackoff — xato (429, 5xx, tarmoq) dan keyingi eng qisqa pauza.
const minBackoff = time.Minute

type MonitorConfig struct {
	// Tick — monitor qanchada bir uyg'onib, vaqti kelgan yo'nalishlarni qidiradi.
	// Har bir yo'nalishning o'z oralig'i jadvaldan (PollingPolicy) olinadi.
	Tick       time.Duration
	MaxBackoff time.Duration // xatolardan keyingi eng uzun pauza
	GroupPause time.Duration // turli yo'nalishlar so'rovlari orasidagi pauza
	// MaxSearchesPerCycle — bir siklda saytga yuboriladigan so'rovlar chegarasi.
	// Ortgan yo'nalishlar keyingi siklga qoladi (eng uzoq tekshirilmaganlari birinchi).
	MaxSearchesPerCycle int
}

// Monitor barcha foydalanuvchilarning faol kuzatuvlarini davriy tekshiradi.
// Bir xil yo'nalish+sana qancha foydalanuvchida bo'lmasin, saytga bitta so'rov ketadi.
// Har bir yo'nalish jo'nashgacha qolgan kunga qarab jadvaldagi oraliqda tekshiriladi.
type Monitor struct {
	repo     WatchRepository
	policies PollingRuleRepository
	users    UserRepository
	searcher TrainSearcher
	notifier Notifier
	clock    Clock
	cfg      MonitorConfig
	log      *slog.Logger
	trigger  chan int64

	running sync.Mutex

	mu          sync.Mutex
	backoff     time.Duration
	pausedUntil time.Time
}

func NewMonitor(repo WatchRepository, policies PollingRuleRepository, users UserRepository, s TrainSearcher, n Notifier, c Clock, cfg MonitorConfig, log *slog.Logger) *Monitor {
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = 30 * time.Minute
	}
	if cfg.Tick == 0 {
		cfg.Tick = domain.MinCheckInterval
	}
	return &Monitor{repo: repo, policies: policies, users: users, searcher: s, notifier: n, clock: c, cfg: cfg, log: log, trigger: make(chan int64, 64)}
}

func (m *Monitor) PausedUntil() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pausedUntil
}

func (m *Monitor) RequestCheck(id int64) {
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
			m.RunCycle(ctx, 0)
			timer.Reset(jitter(m.cfg.Tick))
		case id := <-m.trigger:
			m.RunCycle(ctx, id)
		}
	}
}

type routeGroup struct {
	route     domain.Route
	watches   []domain.Watch
	lastCheck time.Time // guruhdagi eng eski tekshiruv
}

// RunCycle bitta tekshiruv sikli. onlyID != 0 bo'lsa — faqat o'sha kuzatuv.
func (m *Monitor) RunCycle(ctx context.Context, onlyID int64) {
	m.running.Lock()
	defer m.running.Unlock()

	if until := m.PausedUntil(); m.clock.Now().Before(until) {
		m.log.Info("tekshiruv pauzada", "until", until.Format("15:04:05"))
		return
	}

	var watches []domain.Watch
	if onlyID != 0 {
		w, err := m.repo.Get(ctx, onlyID)
		if err != nil {
			return
		}
		if w.Active {
			watches = []domain.Watch{w}
		}
	} else {
		var err error
		if watches, err = m.repo.ListActive(ctx); err != nil {
			m.log.Error("kuzatuvlarni o'qib bo'lmadi", "err", err)
			return
		}
	}

	groups := m.group(ctx, watches)
	if onlyID == 0 {
		groups = m.due(ctx, groups)
	}
	if n := m.cfg.MaxSearchesPerCycle; n > 0 && len(groups) > n {
		m.log.Info("so'rovlar chegarasi — qolganlari keyingi siklda", "routes", len(groups), "limit", n)
		groups = groups[:n]
	}

	chats := map[int64]string{} // sikl ichida foydalanuvchi keshi
	for i, g := range groups {
		if i > 0 && !sleepCtx(ctx, m.cfg.GroupPause) {
			return
		}
		trains, err := m.searcher.Search(ctx, g.route)
		now := m.clock.Now()
		if err != nil {
			d := m.fail(err)
			m.log.Warn("qidiruv xatosi", "route", g.route.Key(), "err", err, "pause", d.Round(time.Second))
			for _, w := range g.watches {
				_, _ = m.repo.Update(ctx, w.ID, func(x *domain.Watch) { x.RecordFailure(now, err) })
			}
			return // qolganlari keyingi siklda — saytni qiynamaymiz
		}
		m.succeed()
		for _, w := range g.watches {
			m.evaluate(ctx, w, trains, now, chats)
		}
	}
}

// group muddati o'tganlarini to'xtatadi va qolganlarini yo'nalish bo'yicha guruhlaydi.
// Eng uzoq tekshirilmagan yo'nalishlar birinchi turadi — chegara bo'lsa ham hech kim "och" qolmaydi.
func (m *Monitor) group(ctx context.Context, watches []domain.Watch) []routeGroup {
	today := m.clock.Today()
	byKey := map[string]*routeGroup{}
	for _, w := range watches {
		if w.IsExpired(today) {
			_, _ = m.repo.Update(ctx, w.ID, func(x *domain.Watch) { x.Expire() })
			continue
		}
		g, ok := byKey[w.Route.Key()]
		if !ok {
			g = &routeGroup{route: w.Route, lastCheck: w.LastCheck}
			byKey[w.Route.Key()] = g
		}
		g.watches = append(g.watches, w)
		if w.LastCheck.Before(g.lastCheck) {
			g.lastCheck = w.LastCheck
		}
	}
	out := make([]routeGroup, 0, len(byKey))
	for _, g := range byKey {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].lastCheck.Before(out[j].lastCheck) })
	return out
}

// due faqat oralig'i o'tgan yo'nalishlarni qoldiradi.
func (m *Monitor) due(ctx context.Context, groups []routeGroup) []routeGroup {
	policy, err := m.policies.Get(ctx)
	if err != nil {
		m.log.Warn("oraliqlar jadvalini o'qib bo'lmadi — standart jadval ishlatiladi", "err", err)
		policy = domain.DefaultPollingPolicy()
	}
	now, today := m.clock.Now(), m.clock.Today()
	out := groups[:0]
	for _, g := range groups {
		if g.lastCheck.IsZero() || now.Sub(g.lastCheck) >= policy.IntervalFor(g.route.Date, today) {
			out = append(out, g)
		}
	}
	return out
}

func (m *Monitor) evaluate(ctx context.Context, w domain.Watch, trains []domain.Train, now time.Time, chats map[int64]string) {
	matches, notify, sig := w.Evaluate(trains)
	var nerr error
	if notify {
		chat, ok := chats[w.UserID]
		if !ok {
			if u, err := m.users.Get(ctx, w.UserID); err == nil {
				chat = u.TelegramChatID
			}
			chats[w.UserID] = chat
		}
		if chat == "" {
			nerr = errors.New("Telegram ulanmagan")
		} else {
			nerr = m.notifier.NotifyAvailability(ctx, chat, w.Route, matches)
		}
		if nerr != nil {
			m.log.Warn("xabar yuborilmadi", "watch", w.ID, "user", w.UserID, "err", nerr)
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
		m.backoff = minBackoff
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
