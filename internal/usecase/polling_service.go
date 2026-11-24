package usecase

import (
	"context"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
)

// PollingService — tekshiruv oraliqlari jadvalini o'qish va (faqat admin) o'zgartirish.
type PollingService struct {
	repo    PollingRuleRepository
	watches WatchRepository
	clock   Clock
	admins  []string
}

func NewPollingService(repo PollingRuleRepository, watches WatchRepository, c Clock, adminIDs []string) *PollingService {
	return &PollingService{repo: repo, watches: watches, clock: c, admins: adminIDs}
}

// Policy — joriy jadval (hamma foydalanuvchi o'qiy oladi: o'z kuzatuvining oralig'ini ko'rish uchun).
func (s *PollingService) Policy(ctx context.Context) (domain.PollingPolicy, error) {
	return s.repo.Get(ctx)
}

// Update jadvalni almashtiradi. Faqat administrator.
func (s *PollingService) Update(ctx context.Context, actor domain.User, rules []domain.PollingRule) (domain.PollingPolicy, error) {
	if !actor.IsAdmin(s.admins) {
		return domain.PollingPolicy{}, domain.ErrForbidden
	}
	p, err := domain.NewPollingPolicy(rules)
	if err != nil {
		return domain.PollingPolicy{}, err
	}
	p.UpdatedAt = s.clock.Now()
	p.UpdatedBy = actor.DisplayName()
	if err := s.repo.Save(ctx, p); err != nil {
		return domain.PollingPolicy{}, err
	}
	return p, nil
}

// TierStats — bitta bosqich bo'yicha yuklama.
type TierStats struct {
	Rule            domain.PollingRule
	Routes          int     // faol yo'nalish+sana juftliklari
	Watches         int     // faol kuzatuvlar
	RequestsPerHour float64 // saytga taxminiy so'rovlar soni
}

// Stats — admin sahifasi uchun: har bir bosqichda nechta yo'nalish va qancha so'rov.
func (s *PollingService) Stats(ctx context.Context, actor domain.User) ([]TierStats, error) {
	if !actor.IsAdmin(s.admins) {
		return nil, domain.ErrForbidden
	}
	p, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	active, err := s.watches.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	return TierLoad(p, active, s.clock.Today()), nil
}

// TierLoad — berilgan jadval bo'yicha kuzatuvlarning bosqichlarga taqsimlanishi.
func TierLoad(p domain.PollingPolicy, active []domain.Watch, today string) []TierStats {
	out := make([]TierStats, len(p.Rules))
	idx := map[int]int{}
	for i, r := range p.Rules {
		out[i].Rule = r
		idx[r.MaxDaysLeft] = i
	}
	routes := map[string]bool{}
	for _, w := range active {
		if w.IsExpired(today) {
			continue
		}
		i := idx[p.RuleFor(domain.DaysLeft(w.Route.Date, today)).MaxDaysLeft]
		out[i].Watches++
		if !routes[w.Route.Key()] {
			routes[w.Route.Key()] = true
			out[i].Routes++
		}
	}
	for i := range out {
		out[i].RequestsPerHour = float64(out[i].Routes) * float64(time.Hour) / float64(out[i].Rule.Interval)
	}
	return out
}
