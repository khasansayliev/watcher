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

type PausedError struct{ Until time.Time }

func (e *PausedError) Error() string {
	return "Sayt xato qaytargani uchun so'rovlar " + e.Until.Format("15:04:05") + " gacha to'xtatilgan"
}

type ThrottledError struct{ Wait time.Duration }

func (e *ThrottledError) Error() string {
	return "Juda tez-tez qidiryapsiz — " + e.Wait.Round(time.Second).String() + " kuting"
}

// SearchService — qo'lda qidiruv. Har bir foydalanuvchiga alohida va umumiy cheklov qo'yiladi.
type SearchService struct {
	searcher  TrainSearcher
	pause     PauseInfo
	clock     Clock
	perUser   time.Duration // bitta foydalanuvchi uchun qidiruvlar orasidagi minimal vaqt
	globalGap time.Duration // butun platforma bo'yicha qo'lda qidiruvlar orasidagi minimal vaqt

	mu         sync.Mutex
	lastByUser map[int64]time.Time
	lastGlobal time.Time
}

func NewSearchService(s TrainSearcher, p PauseInfo, c Clock, perUser, globalGap time.Duration) *SearchService {
	return &SearchService{searcher: s, pause: p, clock: c, perUser: perUser, globalGap: globalGap, lastByUser: map[int64]time.Time{}}
}

func (s *SearchService) Search(ctx context.Context, userID int64, r domain.Route) ([]domain.Train, error) {
	if err := r.Validate(s.clock.Today()); err != nil {
		return nil, err
	}
	now := s.clock.Now()
	if until := s.pause.PausedUntil(); now.Before(until) {
		return nil, &PausedError{Until: until}
	}
	if err := s.reserve(userID, now); err != nil {
		return nil, err
	}
	return s.searcher.Search(ctx, r)
}

func (s *SearchService) reserve(userID int64, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if wait := s.perUser - now.Sub(s.lastByUser[userID]); wait > 0 {
		return &ThrottledError{Wait: wait}
	}
	if wait := s.globalGap - now.Sub(s.lastGlobal); wait > 0 {
		return &ThrottledError{Wait: wait}
	}
	s.lastByUser[userID] = now
	s.lastGlobal = now
	if len(s.lastByUser) > 10000 { // xotira o'smasin
		for id, t := range s.lastByUser {
			if now.Sub(t) > s.perUser {
				delete(s.lastByUser, id)
			}
		}
	}
	return nil
}
