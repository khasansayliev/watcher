package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
)

// CheckRequester — kuzatuvni darhol tekshirishni so'rash (Monitor amalga oshiradi).
type CheckRequester interface {
	RequestCheck(id int64)
}

type CreateWatchInput = domain.NewWatchParams

// CreationGate — foydalanuvchi hozir kuzatuv yarata oladimi (rejim va shaxsiy ruxsat).
// Rad etilsa domain.ValidationError qaytaradi (foydalanuvchiga ko'rsatiladigan sabab bilan).
type CreationGate interface {
	CanCreate(ctx context.Context, u domain.User) error
}

// WatchService — foydalanuvchi o'z kuzatuvlarini boshqaradi. Har bir amal egalikni tekshiradi.
type WatchService struct {
	repo      WatchRepository
	logs      CheckLogRepository
	users     UserRepository
	clock     Clock
	checker   CheckRequester
	gate      CreationGate
	maxActive int
}

func NewWatchService(repo WatchRepository, logs CheckLogRepository, users UserRepository, c Clock, checker CheckRequester, gate CreationGate, maxActivePerUser int) *WatchService {
	return &WatchService{repo: repo, logs: logs, users: users, clock: c, checker: checker, gate: gate, maxActive: maxActivePerUser}
}

func (s *WatchService) Create(ctx context.Context, userID int64, in CreateWatchInput) (domain.Watch, error) {
	owner, err := s.users.Get(ctx, userID)
	if err != nil {
		return domain.Watch{}, err
	}
	if err := s.gate.CanCreate(ctx, owner); err != nil {
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

// CanCreate dashboard uchun: nil — yaratish mumkin, aks holda sabab (ValidationError).
func (s *WatchService) CanCreate(ctx context.Context, userID int64) error {
	owner, err := s.users.Get(ctx, userID)
	if err != nil {
		return err
	}
	return s.gate.CanCreate(ctx, owner)
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
	if w.AdminDisabled {
		return domain.Watch{}, errAdminStopped
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

var errAdminStopped = domain.NewValidationError("Bu kuzatuv administrator tomonidan to'xtatilgan")

func (s *WatchService) RequestCheck(ctx context.Context, userID, id int64) error {
	w, err := s.owned(ctx, userID, id)
	if err != nil {
		return err
	}
	if w.AdminDisabled {
		return errAdminStopped
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

// LogsPerPage — loglar sahifasidagi yozuvlar soni.
const LogsPerPage = 25

// LogPage — kuzatuv loglarining bitta sahifasi (req_time bo'yicha kamayish tartibida).
type LogPage struct {
	Watch   domain.Watch
	Logs    []domain.CheckLog
	Page    int // 1 dan boshlanadi
	Pages   int // kamida 1
	Total   int
	PerPage int
}

// Logs egasining kuzatuvi uchun loglar sahifasini qaytaradi. Chegaradan tashqari sahifa eng yaqin
// to'g'ri sahifaga keltiriladi.
func (s *WatchService) Logs(ctx context.Context, userID, id int64, page int) (LogPage, error) {
	w, err := s.owned(ctx, userID, id)
	if err != nil {
		return LogPage{}, err
	}
	if page < 1 {
		page = 1
	}
	logs, total, err := s.logs.ListByWatch(ctx, id, LogsPerPage, (page-1)*LogsPerPage)
	if err != nil {
		return LogPage{}, err
	}
	pages := max(1, (total+LogsPerPage-1)/LogsPerPage)
	if page > pages { // oxirgi sahifadan keyin so'ralgan — oxirgisini ko'rsatamiz
		page = pages
		if logs, total, err = s.logs.ListByWatch(ctx, id, LogsPerPage, (page-1)*LogsPerPage); err != nil {
			return LogPage{}, err
		}
	}
	return LogPage{Watch: w, Logs: logs, Page: page, Pages: pages, Total: total, PerPage: LogsPerPage}, nil
}

// PruneLogs keepFor dan eski loglarni o'chiradi (baza cheksiz o'smasligi uchun).
func (s *WatchService) PruneLogs(ctx context.Context, keepFor time.Duration) (int64, error) {
	return s.logs.DeleteBefore(ctx, s.clock.Now().Add(-keepFor))
}
