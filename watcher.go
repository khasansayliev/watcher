package main

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxBackoff = 30 * time.Minute

type Watcher struct {
	store    *Store
	rc       *RailwayClient
	tg       *Telegram
	st       *Stations
	interval time.Duration
	buyURL   string
	loc      *time.Location
	trigger  chan string

	mu          sync.Mutex
	backoff     time.Duration
	pausedUntil time.Time
	running     sync.Mutex // bir vaqtda faqat bitta tekshiruv sikli
}

func NewWatcher(store *Store, rc *RailwayClient, tg *Telegram, st *Stations, interval time.Duration, buyURL string) *Watcher {
	loc, err := time.LoadLocation("Asia/Tashkent")
	if err != nil {
		loc = time.FixedZone("UZT", 5*3600)
	}
	return &Watcher{store: store, rc: rc, tg: tg, st: st, interval: interval, buyURL: buyURL, loc: loc, trigger: make(chan string, 16)}
}

func (w *Watcher) Today() string { return time.Now().In(w.loc).Format("2006-01-02") }

func (w *Watcher) Interval() time.Duration { return w.interval }

// PausedUntil — xatolar tufayli tekshiruv qachongacha to'xtatilgan.
func (w *Watcher) PausedUntil() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.pausedUntil
}

// CheckNow — bitta kuzatuvni navbatdan tashqari tekshirish (UI tugmasi).
func (w *Watcher) CheckNow(id string) {
	select {
	case w.trigger <- id:
	default:
	}
}

func (w *Watcher) Run(ctx context.Context) {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			w.cycle(ctx, "")
			timer.Reset(jitter(w.interval))
		case id := <-w.trigger:
			w.cycle(ctx, id)
		}
	}
}

func (w *Watcher) cycle(ctx context.Context, onlyID string) {
	w.running.Lock()
	defer w.running.Unlock()

	if until := w.PausedUntil(); time.Now().Before(until) {
		log.Printf("tekshiruv %s gacha to'xtatilgan (oldingi xato)", until.Format("15:04:05"))
		return
	}

	today := w.Today()
	groups := map[string][]Watch{}
	var keys []string
	for _, wt := range w.store.List() {
		if onlyID != "" && wt.ID != onlyID {
			continue
		}
		if !wt.Active {
			continue
		}
		if wt.Date < today {
			_ = w.store.Update(wt.ID, func(x *Watch) {
				x.Active = false
				x.LastStatus = "Sana o'tib ketdi — to'xtatildi"
			})
			continue
		}
		// Bir xil yo'nalish+sana uchun bitta so'rov yuboriladi.
		k := wt.From + "|" + wt.To + "|" + wt.Date
		if _, ok := groups[k]; !ok {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], wt)
	}

	for i, k := range keys {
		if i > 0 && !sleepCtx(ctx, 2*time.Second+time.Duration(rand.IntN(2000))*time.Millisecond) {
			return
		}
		p := strings.Split(k, "|")
		trains, err := w.rc.Search(ctx, p[0], p[1], p[2])
		now := time.Now()
		if err != nil {
			d := w.fail(err)
			log.Printf("qidiruv xatosi %s: %v (keyingi urinish %s dan keyin)", k, err, d.Round(time.Second))
			for _, wt := range groups[k] {
				_ = w.store.Update(wt.ID, func(x *Watch) {
					x.LastCheck = now
					x.LastStatus = "Xato: " + truncate(err.Error(), 160)
				})
			}
			return // qolganlarini keyingi siklda — saytni qiynamaymiz
		}
		w.succeed()
		for _, wt := range groups[k] {
			w.handle(ctx, wt, trains, now)
		}
	}
}

func (w *Watcher) fail(err error) time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.backoff == 0 {
		w.backoff = w.interval
	} else {
		w.backoff *= 2
	}
	if w.backoff > maxBackoff {
		w.backoff = maxBackoff
	}
	d := w.backoff
	var se *StatusError
	if errors.As(err, &se) && se.RetryAfter > d {
		d = se.RetryAfter
	}
	w.pausedUntil = time.Now().Add(d)
	return d
}

func (w *Watcher) succeed() {
	w.mu.Lock()
	w.backoff = 0
	w.pausedUntil = time.Time{}
	w.mu.Unlock()
}

func (w *Watcher) handle(ctx context.Context, wt Watch, trains []Train, now time.Time) {
	matches := MatchTrains(wt, trains)
	sig := signature(matches)
	// Faqat mos joylar to'plami o'zgarganda xabar yuboramiz (har siklda spam emas).
	notify := len(matches) > 0 && sig != wt.LastSig

	var nerr error
	if notify {
		nerr = w.tg.Send(ctx, wt.ChatID, w.formatMessage(wt, matches))
		if nerr != nil {
			log.Printf("telegram xatosi (%s): %v", wt.ID, nerr)
		}
	}
	_ = w.store.Update(wt.ID, func(x *Watch) {
		x.LastCheck = now
		x.LastMatches = len(matches)
		x.LastStatus = fmt.Sprintf("%d ta poyezd, mos: %d", len(trains), len(matches))
		switch {
		case notify && nerr == nil:
			x.LastSig = sig
			x.LastNotified = now
		case notify:
			x.LastStatus += " | Telegram xato: " + truncate(nerr.Error(), 120)
		case len(matches) == 0:
			x.LastSig = "" // joylar tugadi — qayta chiqsa yana xabar beramiz
		}
	})
}

type Match struct {
	Train Train
	Cars  []Car
}

func MatchTrains(wt Watch, trains []Train) []Match {
	minSeats := wt.MinSeats
	if minSeats < 1 {
		minSeats = 1
	}
	filter := strings.ToLower(strings.TrimSpace(wt.CarType))
	var out []Match
	for _, t := range trains {
		var cars []Car
		for _, c := range t.Cars {
			if c.FreeSeats < minSeats {
				continue
			}
			if wt.MaxPrice > 0 && c.MinPrice > 0 && c.MinPrice > wt.MaxPrice {
				continue
			}
			if filter != "" && !strings.Contains(strings.ToLower(c.Type+" "+c.Class+" "+t.Type+" "+t.Brand), filter) {
				continue
			}
			cars = append(cars, c)
		}
		if len(cars) > 0 {
			out = append(out, Match{Train: t, Cars: cars})
		}
	}
	return out
}

func signature(ms []Match) string {
	set := map[string]bool{}
	for _, m := range ms {
		for _, c := range m.Cars {
			set[m.Train.Number+"|"+m.Train.Departure+"|"+c.Type+"|"+c.Class] = true
		}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ";")
}

func (w *Watcher) formatMessage(wt Watch, ms []Match) string {
	e := html.EscapeString
	var b strings.Builder
	fmt.Fprintf(&b, "🚆 <b>Joy bor!</b> %s → %s, %s\n", e(w.st.Name(wt.From)), e(w.st.Name(wt.To)), wt.Date)
	for _, m := range ms {
		fmt.Fprintf(&b, "\n<b>%s %s</b>  %s → %s", e(m.Train.Number), e(trainLabel(m.Train)), e(m.Train.Departure), e(m.Train.Arrival))
		if m.Train.Duration != "" {
			fmt.Fprintf(&b, " (%s)", e(m.Train.Duration))
		}
		b.WriteString("\n")
		for _, c := range m.Cars {
			fmt.Fprintf(&b, "  • %s: %d joy", e(carLabel(c)), c.FreeSeats)
			if c.MinPrice > 0 {
				fmt.Fprintf(&b, ", %s so'm", formatPrice(c.MinPrice))
			}
			b.WriteString("\n")
		}
		if b.Len() > 3500 { // Telegram limiti 4096
			b.WriteString("\n…")
			break
		}
	}
	fmt.Fprintf(&b, "\nSotib olish: %s", e(w.buyURL))
	return b.String()
}

func jitter(d time.Duration) time.Duration {
	// ±10% — so'rovlar bir xil daqiqaga yopishib qolmasligi uchun
	delta := time.Duration(rand.Int64N(int64(d)/5)) - d/10
	return d + delta
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
