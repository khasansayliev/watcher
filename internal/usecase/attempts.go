package usecase

import (
	"sync"
	"time"
)

// attemptLimiter — muvaffaqiyatsiz urinishlarni sanaydi (xotirada; qayta ishga tushganda tozalanadi).
// Kod bilan kirishda kodni tanlab topishga urinishdan himoya qiladi: kalit (odatda IP) bo'yicha ham,
// umumiy (hamma uchun) ham chegara bor. Kod maydoni 2^40 va amal qilish muddati qisqa, shuning uchun
// umumiy chegara ham tanlab topishni amalda imkonsiz qiladi.
type attemptLimiter struct {
	mu        sync.Mutex
	window    time.Duration
	perKey    int
	global    int
	byKey     map[string][]time.Time
	globalLog []time.Time
}

func newAttemptLimiter(window time.Duration, perKey, global int) *attemptLimiter {
	return &attemptLimiter{window: window, perKey: perKey, global: global, byKey: map[string][]time.Time{}}
}

// allowed — yana urinishga ruxsat bormi (urinishni sanamaydi).
func (l *attemptLimiter) allowed(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(now)
	return len(l.byKey[key]) < l.perKey && len(l.globalLog) < l.global
}

func (l *attemptLimiter) fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(now)
	l.byKey[key] = append(l.byKey[key], now)
	l.globalLog = append(l.globalLog, now)
}

// reset muvaffaqiyatli kirgandan keyin shu kalitning xatolarini unutadi.
func (l *attemptLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byKey, key)
}

func (l *attemptLimiter) prune(now time.Time) {
	cut := now.Add(-l.window)
	l.globalLog = dropBefore(l.globalLog, cut)
	for k, v := range l.byKey {
		if v = dropBefore(v, cut); len(v) == 0 {
			delete(l.byKey, k)
		} else {
			l.byKey[k] = v
		}
	}
}

func dropBefore(ts []time.Time, cut time.Time) []time.Time {
	i := 0
	for i < len(ts) && ts[i].Before(cut) {
		i++
	}
	return ts[i:]
}

// cooldown — kalit bo'yicha "oxirgi marta qachon" (kod so'rash oralig'i uchun).
type cooldown struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func newCooldown() *cooldown { return &cooldown{last: map[string]time.Time{}} }

// try ruxsat bersa true qaytaradi va vaqtni yozib qo'yadi.
func (c *cooldown) try(id string, now time.Time, gap time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.last) > 1000 { // xotira o'sib ketmasin
		for k, t := range c.last {
			if now.Sub(t) > gap {
				delete(c.last, k)
			}
		}
	}
	if t, ok := c.last[id]; ok && now.Sub(t) < gap {
		return false
	}
	c.last[id] = now
	return true
}
