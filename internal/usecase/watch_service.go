package usecase

import (
	"context"
	"fmt"

	"github.com/khasansayliev/watcher/internal/domain"
)

// CheckRequester — kuzatuvni darhol tekshirishni so'rash (Monitor amalga oshiradi).
type CheckRequester interface {
	RequestCheck(id int64)
}

type CreateWatchInput = domain.NewWatchParams

// WatchService — foydalanuvchi o'z kuzatuvlarini boshqaradi. Har bir amal egalikni tekshiradi.
type WatchService struct {
	repo      WatchRepository
	users     UserRepository
	clock     Clock
	checker   CheckRequester
	maxActive int
}

func NewWatchService(repo WatchRepository, users UserRepository, c Clock, checker CheckRequester, maxActivePerUser int) *WatchService {
	return &WatchService{repo: repo, users: users, clock: c, checker: checker, maxActive: maxActivePerUser}
}

func (s *WatchService) Create(ctx context.Context, userID int64, in CreateWatchInput) (domain.Watch, error) {
	owner, err := s.users.Get(ctx, userID)
	if err != nil {
		return domain.Watch{}, err
	}
	w, err := domain.NewWatch(owner, in, s.clock.Today(), s.clock.Now())
	if err != nil {
		return domain.Watch{}, err
	}
	if err := s.checkLimit(ctx, userID); err != nil {
		return domain.Watch{}, err
	}
	saved, err := s.repo.Create(ctx, w)
	if err != nil {
		return domain.Watch{}, err
	}
	s.checker.RequestCheck(saved.ID)
	return saved, nil
}

func (s *WatchService) List(ctx context.Context, userID int64) ([]domain.Watch, error) {
	return s.repo.ListByUser(ctx, userID)
}

func (s *WatchService) Delete(ctx context.Context, userID, id int64) error {
	if _, err := s.owned(ctx, userID, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

func (s *WatchService) Toggle(ctx context.Context, userID, id int64) (domain.Watch, error) {
	w, err := s.owned(ctx, userID, id)
	if err != nil {
		return domain.Watch{}, err
	}
	if !w.Active { // yoqilmoqchi — limitni tekshiramiz
		if err := s.checkLimit(ctx, userID); err != nil {
			return domain.Watch{}, err
		}
	}
	w, err = s.repo.Update(ctx, id, func(x *domain.Watch) { x.Toggle() })
	if err != nil {
		return domain.Watch{}, err
	}
	if w.Active {
		s.checker.RequestCheck(id)
	}
	return w, nil
}

func (s *WatchService) RequestCheck(ctx context.Context, userID, id int64) error {
	if _, err := s.owned(ctx, userID, id); err != nil {
		return err
	}
	s.checker.RequestCheck(id)
	return nil
}

func (s *WatchService) owned(ctx context.Context, userID, id int64) (domain.Watch, error) {
	w, err := s.repo.Get(ctx, id)
	if err != nil {
		return domain.Watch{}, err
	}
	if !w.OwnedBy(userID) {
		// Mavjudligini oshkor qilmaymiz.
		return domain.Watch{}, domain.ErrNotFound
	}
	return w, nil
}

func (s *WatchService) checkLimit(ctx context.Context, userID int64) error {
	if s.maxActive <= 0 {
		return nil
	}
	n, err := s.repo.CountActiveByUser(ctx, userID)
	if err != nil {
		return err
	}
	if n >= s.maxActive {
		return domain.NewValidationError(fmt.Sprintf("Bir vaqtda ko'pi bilan %d ta faol kuzatuv bo'lishi mumkin", s.maxActive))
	}
	return nil
}
