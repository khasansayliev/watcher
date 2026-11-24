package domain

import "time"

// CreationMode — kuzatuv yaratish kimga ruxsat etilgani (admin boshqaradi).
type CreationMode string

const (
	// CreationApproved — faqat admin ruxsat bergan foydalanuvchilar yarata oladi (standart).
	CreationApproved CreationMode = "approved"
	// CreationAll — hamma yarata oladi.
	CreationAll CreationMode = "all"
	// CreationNone — hech kim yarata olmaydi (adminlardan tashqari).
	CreationNone CreationMode = "none"
)

func (m CreationMode) Valid() bool {
	return m == CreationApproved || m == CreationAll || m == CreationNone
}

// Settings — butun platforma sozlamalari.
type Settings struct {
	CreationMode CreationMode
}

func DefaultSettings() Settings { return Settings{CreationMode: CreationApproved} }

// CheckCreate foydalanuvchi hozir yangi kuzatuv yarata oladimi — aytadi. Adminlar har doim yarata oladi.
func (s Settings) CheckCreate(u User, isAdmin bool) error {
	if isAdmin {
		return nil
	}
	switch s.CreationMode {
	case CreationAll:
		return nil
	case CreationNone:
		return NewValidationError("Yangi kuzatuv yaratish hozircha administrator tomonidan to'xtatilgan")
	default:
		if u.CanCreate {
			return nil
		}
		return NewValidationError("Kuzatuv yaratish uchun administrator ruxsati kerak. Administratorga murojaat qiling")
	}
}

// UserSummary — admin ro'yxati uchun foydalanuvchi va uning kuzatuvlari statistikasi.
type UserSummary struct {
	User
	LastLoginAt   time.Time
	ActiveWatches int // hozir tekshirilayotganlar
	TotalWatches  int
}

// WatchWithOwner — admin ro'yxati uchun kuzatuv va egasi.
type WatchWithOwner struct {
	Watch
	Owner User
}
