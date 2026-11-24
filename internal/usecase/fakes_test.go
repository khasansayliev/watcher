package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

// Fake portlar: usecase'lar tarmoqsiz va bazasiz, to'liq izolyatsiyada tekshiriladi.

type memWatches struct {
	mu sync.Mutex
	ws []domain.Watch
	n  int64
}

func (r *memWatches) Create(_ context.Context, w domain.Watch) (domain.Watch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n++
	w.ID = r.n
	r.ws = append(r.ws, w)
	return w, nil
}
func (r *memWatches) Get(_ context.Context, id int64) (domain.Watch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, w := range r.ws {
		if w.ID == id {
			return w, nil
		}
	}
	return domain.Watch{}, domain.ErrNotFound
}
func (r *memWatches) ListByUser(_ context.Context, uid int64) ([]domain.Watch, error) {
	return r.filter(func(w domain.Watch) bool { return w.UserID == uid }), nil
}
func (r *memWatches) ListActive(context.Context) ([]domain.Watch, error) {
	return r.filter(func(w domain.Watch) bool { return w.Active }), nil
}
func (r *memWatches) CountActiveByUser(_ context.Context, uid int64) (int, error) {
	return len(r.filter(func(w domain.Watch) bool { return w.Active && w.UserID == uid })), nil
}
func (r *memWatches) Update(_ context.Context, id int64, fn func(*domain.Watch)) (domain.Watch, error) {
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
func (r *memWatches) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.ws {
		if r.ws[i].ID == id {
			r.ws = append(r.ws[:i], r.ws[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}
func (r *memWatches) filter(f func(domain.Watch) bool) []domain.Watch {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Watch
	for _, w := range r.ws {
		if f(w) {
			out = append(out, w)
		}
	}
	return out
}

type memUsers struct {
	mu sync.Mutex
	m  map[int64]domain.User
	n  int64
}

func newMemUsers() *memUsers { return &memUsers{m: map[int64]domain.User{}} }

func (r *memUsers) add(u domain.User) domain.User {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n++
	u.ID = r.n
	r.m[u.ID] = u
	return u
}
func (r *memUsers) UpsertByIdentity(_ context.Context, id domain.Identity, now time.Time) (domain.User, error) {
	r.mu.Lock()
	for _, u := range r.m {
		if u.TelegramID == id.TelegramID {
			u.Username, u.Name = id.Username, id.FullName()
			r.m[u.ID] = u
			r.mu.Unlock()
			return u, nil
		}
	}
	r.mu.Unlock()
	return r.add(domain.User{TelegramID: id.TelegramID, Username: id.Username, Name: id.FullName(), CreatedAt: now}), nil
}
func (r *memUsers) Get(_ context.Context, id int64) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.m[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return u, nil
}

type session struct {
	user    int64
	expires time.Time
}

type memSessions struct {
	mu sync.Mutex
	m  map[string]session
	u  *memUsers
}

func (r *memSessions) Create(_ context.Context, h []byte, uid int64, exp time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[string(h)] = session{uid, exp}
	return nil
}
func (r *memSessions) UserByToken(ctx context.Context, h []byte, now time.Time) (domain.User, error) {
	r.mu.Lock()
	s, ok := r.m[string(h)]
	r.mu.Unlock()
	if !ok || !s.expires.After(now) {
		return domain.User{}, domain.ErrUnauthorized
	}
	return r.u.Get(ctx, s.user)
}
func (r *memSessions) Delete(_ context.Context, h []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, string(h))
	return nil
}
func (r *memSessions) DeleteExpired(_ context.Context, now time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for k, s := range r.m {
		if !s.expires.After(now) {
			delete(r.m, k)
			n++
		}
	}
	return n, nil
}

type memPolicy struct {
	mu sync.Mutex
	p  domain.PollingPolicy
}

func (r *memPolicy) Get(context.Context) (domain.PollingPolicy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.p, nil
}
func (r *memPolicy) Save(_ context.Context, p domain.PollingPolicy) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.p = p
	return nil
}

type fakeSearcher struct {
	mu     sync.Mutex
	calls  int
	trains []domain.Train
	err    error
}

func (f *fakeSearcher) Search(context.Context, domain.Route) ([]domain.Train, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.trains, f.err
}

type sentMsg struct{ chat, text string }

type fakeNotifier struct {
	mu      sync.Mutex
	sent    []sentMsg
	blocked map[string]bool // bu chatlar botni bloklagan
}

func (f *fakeNotifier) Enabled() bool { return true }
func (f *fakeNotifier) NotifyAvailability(_ context.Context, chat string, _ domain.Route, _ []domain.Match) error {
	return f.SendText(context.Background(), chat, "joy bor")
}
func (f *fakeNotifier) SendText(_ context.Context, chat, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.blocked[chat] {
		return errors.New("telegram: Forbidden: bot was blocked by the user")
	}
	f.sent = append(f.sent, sentMsg{chat, text})
	return nil
}
func (f *fakeNotifier) count(chat string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, m := range f.sent {
		if m.chat == chat {
			n++
		}
	}
	return n
}

// fakeVerifier — imzo tekshiruvini simulyatsiya qiladi: valid=false bo'lsa ErrBadSignature.
type fakeVerifier struct {
	ident domain.Identity
	valid bool
}

func (f *fakeVerifier) Verify(map[string]string) (domain.Identity, error) {
	if !f.valid {
		return domain.Identity{}, usecase.ErrBadSignature
	}
	return f.ident, nil
}

type fixedClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fixedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *fixedClock) Today() string { return c.Now().Format(domain.DateLayout) }
func (c *fixedClock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// env — barcha usecase'lar fake portlar bilan ulangan.
type env struct {
	watches  *memWatches
	users    *memUsers
	sessions *memSessions
	search   *fakeSearcher
	notif    *fakeNotifier
	clock    *fixedClock
	verifier *fakeVerifier
	policy   *memPolicy

	monitor  *usecase.Monitor
	ws       *usecase.WatchService
	auth     *usecase.AuthService
	tg       *usecase.TelegramService
	searchUC *usecase.SearchService
	polling  *usecase.PollingService
}

var sample = []domain.Train{{Number: "764Ф", Cars: []domain.Car{{Type: "Сидячий", FreeSeats: 5, Price: 270000}}}}

func newEnv(maxPerUser, maxSearches int) *env {
	e := &env{
		watches:  &memWatches{},
		users:    newMemUsers(),
		search:   &fakeSearcher{trains: sample},
		notif:    &fakeNotifier{},
		clock:    &fixedClock{t: time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)},
		verifier: &fakeVerifier{valid: true},
		policy:   &memPolicy{p: domain.DefaultPollingPolicy()},
	}
	e.sessions = &memSessions{m: map[string]session{}, u: e.users}
	log := discardLog()
	e.monitor = usecase.NewMonitor(e.watches, e.policy, e.users, e.search, e.notif, e.clock,
		usecase.MonitorConfig{Tick: 10 * time.Second, MaxSearchesPerCycle: maxSearches}, log)
	e.ws = usecase.NewWatchService(e.watches, e.users, e.clock, e.monitor, maxPerUser)
	e.auth = usecase.NewAuthService(e.verifier, e.users, e.sessions, e.clock, 24*time.Hour)
	e.tg = usecase.NewTelegramService(e.users, e.notif, "https://watcher.test")
	e.searchUC = usecase.NewSearchService(e.search, e.monitor, e.clock, 10*time.Second, 0)
	e.polling = usecase.NewPollingService(e.policy, e.watches, e.clock, []int64{adminTG})
	return e
}

// adminTG — testlardagi administratorning Telegram ID'si.
const adminTG = 999

var tgSeq int64 = 1000

func nextTG() int64 { tgSeq++; return tgSeq }

func (e *env) admin() domain.User {
	return e.users.add(domain.User{Username: "admin", TelegramID: adminTG})
}

func (e *env) user(name string) domain.User {
	return e.users.add(domain.User{Username: name, TelegramID: nextTG()})
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
