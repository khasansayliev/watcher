package usecase_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/khasansayliev/railbot/internal/domain"
	"github.com/khasansayliev/railbot/internal/usecase"
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
		if u.GoogleSub == id.Subject {
			u.Email, u.Name = id.Email, id.Name
			r.m[u.ID] = u
			r.mu.Unlock()
			return u, nil
		}
	}
	r.mu.Unlock()
	return r.add(domain.User{GoogleSub: id.Subject, Email: id.Email, Name: id.Name, CreatedAt: now}), nil
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
func (r *memUsers) SetTelegramChat(_ context.Context, id int64, chat string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, u := range r.m {
		if chat != "" && u.TelegramChatID == chat && k != id {
			u.TelegramChatID = ""
			r.m[k] = u
		}
	}
	u, ok := r.m[id]
	if !ok {
		return domain.ErrNotFound
	}
	u.TelegramChatID = chat
	r.m[id] = u
	return nil
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

type memCodes struct {
	mu sync.Mutex
	m  map[string]session
}

func (r *memCodes) Create(_ context.Context, code string, uid int64, exp time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[code] = session{uid, exp}
	return nil
}
func (r *memCodes) Consume(_ context.Context, code string, now time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.m[code]
	delete(r.m, code)
	if !ok || !s.expires.After(now) {
		return 0, domain.ErrNotFound
	}
	return s.user, nil
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
	mu   sync.Mutex
	sent []sentMsg
}

func (f *fakeNotifier) Enabled() bool { return true }
func (f *fakeNotifier) NotifyAvailability(_ context.Context, chat string, _ domain.Route, _ []domain.Match) error {
	return f.SendText(context.Background(), chat, "joy bor")
}
func (f *fakeNotifier) SendText(_ context.Context, chat, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sentMsg{chat, text})
	return nil
}
func (f *fakeNotifier) LinkURL(code string) (string, error) {
	return "https://t.me/testbot?start=" + code, nil
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

type fakeIDP struct{ ident domain.Identity }

func (f *fakeIDP) AuthURL(state string) string { return "https://accounts.example/auth?state=" + state }
func (f *fakeIDP) Exchange(context.Context, string) (domain.Identity, error) {
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
	codes    *memCodes
	search   *fakeSearcher
	notif    *fakeNotifier
	clock    *fixedClock
	idp      *fakeIDP

	monitor  *usecase.Monitor
	ws       *usecase.WatchService
	auth     *usecase.AuthService
	tg       *usecase.TelegramService
	searchUC *usecase.SearchService
}

var sample = []domain.Train{{Number: "764Ф", Cars: []domain.Car{{Type: "Сидячий", FreeSeats: 5, Price: 270000}}}}

func newEnv(maxPerUser, maxSearches int) *env {
	e := &env{
		watches: &memWatches{},
		users:   newMemUsers(),
		codes:   &memCodes{m: map[string]session{}},
		search:  &fakeSearcher{trains: sample},
		notif:   &fakeNotifier{},
		clock:   &fixedClock{t: time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)},
		idp:     &fakeIDP{},
	}
	e.sessions = &memSessions{m: map[string]session{}, u: e.users}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	e.monitor = usecase.NewMonitor(e.watches, e.users, e.search, e.notif, e.clock,
		usecase.MonitorConfig{Interval: 5 * time.Minute, MaxSearchesPerCycle: maxSearches}, log)
	e.ws = usecase.NewWatchService(e.watches, e.users, e.clock, e.monitor, maxPerUser)
	e.auth = usecase.NewAuthService(e.idp, e.users, e.sessions, e.clock, 24*time.Hour)
	e.tg = usecase.NewTelegramService(e.users, e.codes, e.notif, e.clock)
	e.searchUC = usecase.NewSearchService(e.search, e.monitor, e.clock, 10*time.Second, 0)
	return e
}

func (e *env) user(chat string) domain.User {
	return e.users.add(domain.User{Email: chat + "@example.com", TelegramChatID: chat})
}
