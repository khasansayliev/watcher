package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

// Fake portlar: usecase'lar tarmoqsiz va bazasiz, to'liq izolyatsiyada tekshiriladi.

type memWatches struct {
	mu    sync.Mutex
	ws    []domain.Watch
	n     int64
	users *memUsers // o'chirilgan foydalanuvchilar kuzatuvlarini ListActive'dan chiqarish uchun
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
	return r.filter(func(w domain.Watch) bool {
		if !w.Runnable() {
			return false
		}
		if r.users != nil {
			if u, err := r.users.Get(context.Background(), w.UserID); err == nil && u.Disabled {
				return false
			}
		}
		return true
	}), nil
}

func (r *memWatches) ListAll(_ context.Context, userID int64, limit, offset int) ([]domain.WatchWithOwner, int, error) {
	all := r.filter(func(w domain.Watch) bool { return userID == 0 || w.UserID == userID })
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID }) // eng yangisi birinchi
	total := len(all)
	offset = min(offset, total)
	end := min(offset+limit, total)
	var out []domain.WatchWithOwner
	for _, w := range all[offset:end] {
		x := domain.WatchWithOwner{Watch: w}
		if r.users != nil {
			x.Owner, _ = r.users.Get(context.Background(), w.UserID)
		}
		out = append(out, x)
	}
	return out, total, nil
}
func (r *memWatches) CountActiveByUser(_ context.Context, uid int64) (int, error) {
	return len(r.filter(func(w domain.Watch) bool { return w.Runnable() && w.UserID == uid })), nil
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
func (r *memUsers) UpsertByTelegram(_ context.Context, p domain.TelegramProfile, now time.Time) (domain.User, error) {
	r.mu.Lock()
	for _, u := range r.m {
		if u.TelegramChatID == p.ID {
			u.Username, u.Name = p.Username, p.Name
			r.m[u.ID] = u
			r.mu.Unlock()
			return u, nil
		}
	}
	r.mu.Unlock()
	return r.add(domain.User{TelegramChatID: p.ID, Username: p.Username, Name: p.Name, CreatedAt: now}), nil
}
func (r *memUsers) List(_ context.Context, limit, offset int) ([]domain.UserSummary, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var all []domain.User
	for _, u := range r.m {
		all = append(all, u)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	total := len(all)
	offset = min(offset, total)
	var out []domain.UserSummary
	for _, u := range all[offset:min(offset+limit, total)] {
		out = append(out, domain.UserSummary{User: u})
	}
	return out, total, nil
}

func (r *memUsers) SetCanCreate(_ context.Context, id int64, allow bool) error {
	return r.mutate(id, func(u *domain.User) { u.CanCreate = allow })
}

func (r *memUsers) SetDisabled(_ context.Context, id int64, disabled bool) error {
	return r.mutate(id, func(u *domain.User) { u.Disabled = disabled })
}

func (r *memUsers) mutate(id int64, fn func(*domain.User)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.m[id]
	if !ok {
		return domain.ErrNotFound
	}
	fn(&u)
	r.m[id] = u
	return nil
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
func (r *memSessions) DeleteByUser(_ context.Context, uid int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, ses := range r.m {
		if ses.user == uid {
			delete(r.m, k)
		}
	}
	return nil
}
func (r *memSessions) DeleteByTelegram(_ context.Context, tg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, ses := range r.m {
		if u, err := r.u.Get(context.Background(), ses.user); err == nil && u.TelegramChatID == tg {
			delete(r.m, k)
		}
	}
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

type loginReq struct {
	user    int64 // 0 = tasdiqlanmagan
	expires time.Time
}

type memLogins struct {
	mu sync.Mutex
	m  map[string]loginReq
}

func (r *memLogins) Begin(_ context.Context, code string, exp time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[code] = loginReq{expires: exp}
	return nil
}
func (r *memLogins) Approve(_ context.Context, code string, uid int64, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	q, ok := r.m[code]
	if !ok || q.user != 0 || !q.expires.After(now) {
		return domain.ErrNotFound
	}
	q.user = uid
	r.m[code] = q
	return nil
}
func (r *memLogins) Claim(_ context.Context, code string, now time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	q, ok := r.m[code]
	if !ok || !q.expires.After(now) {
		return 0, domain.ErrNotFound
	}
	if q.user == 0 {
		return 0, usecase.ErrLoginPending
	}
	delete(r.m, code)
	return q.user, nil
}

type memSettings struct {
	mu sync.Mutex
	s  domain.Settings
}

func (r *memSettings) Get(context.Context) (domain.Settings, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.s, nil
}
func (r *memSettings) SetCreationMode(_ context.Context, m domain.CreationMode) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.s.CreationMode = m
	return nil
}

type memLogs struct {
	mu   sync.Mutex
	logs []domain.CheckLog
	n    int64
}

func (r *memLogs) Add(_ context.Context, l domain.CheckLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n++
	l.ID = r.n
	r.logs = append(r.logs, l)
	return nil
}

// ListByWatch req_time bo'yicha kamayish tartibida (teng bo'lsa id bo'yicha).
func (r *memLogs) ListByWatch(_ context.Context, id int64, limit, offset int) ([]domain.CheckLog, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var all []domain.CheckLog
	for _, l := range r.logs {
		if l.WatchID == id {
			all = append(all, l)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if !all[i].ReqTime.Equal(all[j].ReqTime) {
			return all[i].ReqTime.After(all[j].ReqTime)
		}
		return all[i].ID > all[j].ID
	})
	total := len(all)
	if offset > total {
		offset = total
	}
	end := min(offset+limit, total)
	return all[offset:end], total, nil
}

func (r *memLogs) DeleteBefore(_ context.Context, t time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var keep []domain.CheckLog
	var n int64
	for _, l := range r.logs {
		if l.ReqTime.Before(t) {
			n++
		} else {
			keep = append(keep, l)
		}
	}
	r.logs = keep
	return n, nil
}

func (r *memLogs) forWatch(id int64) []domain.CheckLog {
	l, _, _ := r.ListByWatch(context.Background(), id, 1000, 0)
	return l
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
	// onSearch — so'rov paytida chaqiriladi (poyga holatlarini sinash uchun).
	onSearch func()
}

func (f *fakeSearcher) Search(context.Context, domain.Route) ([]domain.Train, error) {
	f.mu.Lock()
	f.calls++
	hook := f.onSearch
	trains, err := f.trains, f.err
	f.mu.Unlock()
	if hook != nil {
		hook() // so'rov yuborilgan paytda sodir bo'ladigan voqea (masalan, tahrirlash)
	}
	return trains, err
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
func (f *fakeNotifier) BotURL(code string) (string, error) {
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
	logs     *memLogs
	settings *memSettings
	users    *memUsers
	sessions *memSessions
	logins   *memLogins
	search   *fakeSearcher
	notif    *fakeNotifier
	clock    *fixedClock
	policy   *memPolicy

	monitor  *usecase.Monitor
	ws       *usecase.WatchService
	auth     *usecase.AuthService
	tg       *usecase.TelegramService
	searchUC *usecase.SearchService
	polling  *usecase.PollingService
	admin    *usecase.AdminService
}

var sample = []domain.Train{{Number: "764Ф", Cars: []domain.Car{{Type: "Сидячий", FreeSeats: 5, Price: 270000}}}}

func newEnv(maxPerUser, maxSearches int) *env {
	e := &env{
		watches:  &memWatches{},
		logs:     &memLogs{},
		settings: &memSettings{s: domain.DefaultSettings()},
		users:    newMemUsers(),
		logins:   &memLogins{m: map[string]loginReq{}},
		search:   &fakeSearcher{trains: sample},
		notif:    &fakeNotifier{},
		clock:    &fixedClock{t: time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)},
		policy:   &memPolicy{p: domain.DefaultPollingPolicy()},
	}
	e.sessions = &memSessions{m: map[string]session{}, u: e.users}
	e.watches.users = e.users
	log := discardLog()
	e.admin = usecase.NewAdminService(e.users, e.watches, e.settings, e.sessions, []string{"900"})
	e.monitor = usecase.NewMonitor(e.watches, e.logs, e.policy, e.users, e.admin, e.search, e.notif, e.clock,
		usecase.MonitorConfig{Tick: 10 * time.Second, MaxSearchesPerCycle: maxSearches}, log)
	e.ws = usecase.NewWatchService(e.watches, e.logs, e.users, e.clock, e.monitor, e.admin, maxPerUser)
	e.auth = usecase.NewAuthService(e.users, e.logins, e.sessions, e.notif, e.clock, 24*time.Hour)
	e.tg = usecase.NewTelegramService(e.users, e.notif)
	e.searchUC = usecase.NewSearchService(e.search, e.monitor, e.clock, 10*time.Second, 0)
	e.polling = usecase.NewPollingService(e.policy, e.watches, e.clock, []string{"900"})
	return e
}

func (e *env) user(chat string) domain.User {
	return e.users.add(domain.User{Name: chat, TelegramChatID: chat, CanCreate: true})
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
