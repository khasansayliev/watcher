package usecase

import (
	"context"
	"strings"

	"github.com/khasansayliev/railbot/internal/domain"
)

// CheckRequester — kuzatuvni darhol tekshirishni so'rash (Monitor amalga oshiradi).
type CheckRequester interface {
	RequestCheck(id string)
}

type CreateWatchInput = domain.NewWatchParams

// WatchService — kuzatuvlarni boshqarish stsenariylari.
type WatchService struct {
	repo     WatchRepository
	notifier Notifier
	clock    Clock
	checker  CheckRequester
}

func NewWatchService(repo WatchRepository, n Notifier, c Clock, checker CheckRequester) *WatchService {
	return &WatchService{repo: repo, notifier: n, clock: c, checker: checker}
}

func (s *WatchService) Create(ctx context.Context, in CreateWatchInput) (domain.Watch, error) {
	w, err := domain.NewWatch(in, s.clock.Today(), s.clock.Now())
	if err != nil {
		return domain.Watch{}, err
	}
	saved, err := s.repo.Create(ctx, w)
	if err != nil {
		return domain.Watch{}, err
	}
	s.checker.RequestCheck(saved.ID)
	return saved, nil
}

func (s *WatchService) List(ctx context.Context) ([]domain.Watch, error) {
	return s.repo.List(ctx)
}

func (s *WatchService) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *WatchService) Toggle(ctx context.Context, id string) (domain.Watch, error) {
	w, err := s.repo.Update(ctx, id, func(x *domain.Watch) { x.Toggle() })
	if err != nil {
		return domain.Watch{}, err
	}
	if w.Active {
		s.checker.RequestCheck(id)
	}
	return w, nil
}

func (s *WatchService) RequestCheck(ctx context.Context, id string) error {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return err
	}
	s.checker.RequestCheck(id)
	return nil
}

func (s *WatchService) SendTest(ctx context.Context, chatID string) error {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return domain.NewValidationError("Chat ID kiriting")
	}
	return s.notifier.SendTest(ctx, chatID)
}
