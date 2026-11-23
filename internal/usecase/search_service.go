package usecase

import (
	"context"
	"sync"
	"time"

	"github.com/khasansayliev/railbot/internal/domain"
)

// PauseInfo — tashqi API xatolari sababli pauza holati (Monitor amalga oshiradi).
type PauseInfo interface {
	PausedUntil() time.Time
}

// PausedError — so'rovlar vaqtincha to'xtatilgan.
type PausedError struct{ Until time.Time }

func (e *PausedError) Error() string {
	return "Sayt xato qaytargani uchun so'rovlar " + e.Until.Format("15:04:05") + " gacha to'xtatilgan"
}

// ThrottledError — qo'lda qidiruv juda tez-tez.
type ThrottledError struct{ Wait time.Duration }

func (e *ThrottledError) Error() string {
	return "Juda tez-tez qidiryapsiz — " + e.Wait.Round(time.Second).String() + " kuting"
}

// SearchService — qo'lda qidiruv. Saytga yuklama bermaslik uchun cheklangan.
type SearchService struct {
	searcher TrainSearcher
	pause    PauseInfo
	clock    Clock
	minGap   time.Duration

	mu   sync.Mutex
	last time.Time
}

func NewSearchService(s TrainSearcher, p PauseInfo, c Clock, minGap time.Duration) *SearchService {
	return &SearchService{searcher: s, pause: p, clock: c, minGap: minGap}
}

func (s *SearchService) Search(ctx context.Context, r domain.Route) ([]domain.Train, error) {
	if err := r.Validate(s.clock.Today()); err != nil {
		return nil, err
	}
	now := s.clock.Now()
	if until := s.pause.PausedUntil(); now.Before(until) {
		return nil, &PausedError{Until: until}
	}
	s.mu.Lock()
	if wait := s.minGap - now.Sub(s.last); wait > 0 {
		s.mu.Unlock()
		return nil, &ThrottledError{Wait: wait}
	}
	s.last = now
	s.mu.Unlock()
	return s.searcher.Search(ctx, r)
}
