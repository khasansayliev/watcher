package usecase

import (
	"context"

	"github.com/khasansayliev/watcher/internal/domain"
)

// AdminPerPage — admin ro'yxatlaridagi yozuvlar soni (bir sahifada).
const AdminPerPage = 20

// Pager — sahifalash ma'lumoti.
type Pager struct {
	Page, Pages, Total, PerPage int
}

// paginate sahifa raqamini [1, pages] oralig'iga keltiradi.
func paginate(page, total, per int) Pager {
	pages := max(1, (total+per-1)/per)
	return Pager{Page: min(max(page, 1), pages), Pages: pages, Total: total, PerPage: per}
}

type UserPage struct {
	Pager
	Users    []domain.UserSummary
	Settings domain.Settings
}

type AdminWatchPage struct {
	Pager
	Watches []domain.WatchWithOwner
	Owner   *domain.User // faqat bitta foydalanuvchi bo'yicha filtrlanganda
}

// AdminService — administrator amallari: foydalanuvchilar, ularning kuzatuvlari va kuzatuv yaratish
// ruxsati. Har bir amal avval chaqiruvchi administrator ekanini tekshiradi (domain.ErrForbidden).
// Shuningdek, kuzatuv yaratish qoidasini (CanCreate) WatchService uchun taqdim etadi.
type AdminService struct {
	users    UserRepository
	watches  WatchRepository
	settings SettingsRepository
	sessions SessionRepository
	admins   []string
}

func NewAdminService(users UserRepository, watches WatchRepository, settings SettingsRepository, sessions SessionRepository, adminIDs []string) *AdminService {
	return &AdminService{users: users, watches: watches, settings: settings, sessions: sessions, admins: adminIDs}
}

func (s *AdminService) require(actor domain.User) error {
	if !actor.IsAdmin(s.admins) {
		return domain.ErrForbidden
	}
	return nil
}

// CanCreate foydalanuvchi hozir kuzatuv yarata oladimi (rejim + shaxsiy ruxsat). Admin har doim yarata oladi.
func (s *AdminService) CanCreate(ctx context.Context, u domain.User) error {
	settings, err := s.settings.Get(ctx)
	if err != nil {
		return err
	}
	return settings.CheckCreate(u, u.IsAdmin(s.admins))
}

func (s *AdminService) Users(ctx context.Context, actor domain.User, page int) (UserPage, error) {
	if err := s.require(actor); err != nil {
		return UserPage{}, err
	}
	settings, err := s.settings.Get(ctx)
	if err != nil {
		return UserPage{}, err
	}
	_, total, err := s.users.List(ctx, 1, 0)
	if err != nil {
		return UserPage{}, err
	}
	pg := paginate(page, total, AdminPerPage)
	list, _, err := s.users.List(ctx, AdminPerPage, (pg.Page-1)*AdminPerPage)
	if err != nil {
		return UserPage{}, err
	}
	return UserPage{Pager: pg, Users: list, Settings: settings}, nil
}

func (s *AdminService) SetCanCreate(ctx context.Context, actor domain.User, userID int64, allow bool) error {
	if err := s.require(actor); err != nil {
		return err
	}
	return s.users.SetCanCreate(ctx, userID, allow)
}

// SetUserDisabled akkauntni o'chiradi/yoqadi. O'chirilganda barcha sessiyalar tugaydi va
// foydalanuvchining kuzatuvlari tekshirilmay qoladi (qayta yoqilsa davom etadi).
// O'zini o'chirib bo'lmaydi: admin tizimdan chiqib ketmasligi uchun.
func (s *AdminService) SetUserDisabled(ctx context.Context, actor domain.User, userID int64, disabled bool) error {
	if err := s.require(actor); err != nil {
		return err
	}
	if disabled && actor.ID == userID {
		return domain.NewValidationError("O'zingizning akkauntingizni o'chira olmaysiz")
	}
	if err := s.users.SetDisabled(ctx, userID, disabled); err != nil {
		return err
	}
	if disabled {
		return s.sessions.DeleteByUser(ctx, userID)
	}
	return nil
}

// Watches barcha kuzatuvlarni (yoki bitta foydalanuvchiniki) qaytaradi, eng yangisi birinchi.
func (s *AdminService) Watches(ctx context.Context, actor domain.User, userID int64, page int) (AdminWatchPage, error) {
	if err := s.require(actor); err != nil {
		return AdminWatchPage{}, err
	}
	out := AdminWatchPage{}
	if userID != 0 {
		u, err := s.users.Get(ctx, userID)
		if err != nil {
			return AdminWatchPage{}, err
		}
		out.Owner = &u
	}
	_, total, err := s.watches.ListAll(ctx, userID, 1, 0)
	if err != nil {
		return AdminWatchPage{}, err
	}
	out.Pager = paginate(page, total, AdminPerPage)
	if out.Watches, _, err = s.watches.ListAll(ctx, userID, AdminPerPage, (out.Page-1)*AdminPerPage); err != nil {
		return AdminWatchPage{}, err
	}
	return out, nil
}

// SetWatchDisabled kuzatuvni admin nomidan to'xtatadi/yoqadi. To'xtatilganini foydalanuvchi o'zi yoqa olmaydi.
func (s *AdminService) SetWatchDisabled(ctx context.Context, actor domain.User, watchID int64, disabled bool) error {
	if err := s.require(actor); err != nil {
		return err
	}
	_, err := s.watches.Update(ctx, watchID, func(w *domain.Watch) {
		w.AdminDisabled = disabled
		if !disabled {
			w.LastSig = "" // qayta yoqilganda keyingi topilma albatta xabar qilinsin
		} else {
			w.LastStatus = "Administrator tomonidan to'xtatilgan"
		}
	})
	return err
}

func (s *AdminService) CreationMode(ctx context.Context, actor domain.User) (domain.Settings, error) {
	if err := s.require(actor); err != nil {
		return domain.Settings{}, err
	}
	return s.settings.Get(ctx)
}

func (s *AdminService) SetCreationMode(ctx context.Context, actor domain.User, m domain.CreationMode) error {
	if err := s.require(actor); err != nil {
		return err
	}
	if !m.Valid() {
		return domain.NewValidationError("Noto'g'ri rejim")
	}
	return s.settings.SetCreationMode(ctx, m)
}

var _ CreationGate = (*AdminService)(nil)
