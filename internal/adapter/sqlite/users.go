package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

type Users struct{ db *sql.DB }

var _ usecase.UserRepository = (*Users)(nil)

func NewUsers(db *sql.DB) *Users { return &Users{db: db} }

<<<<<<< Updated upstream
const userCols = `id, telegram_id, username, name, photo_url, created_at`
=======
const userCols = `id, telegram_id, username, name, created_at`
>>>>>>> Stashed changes

type rowScanner interface{ Scan(dest ...any) error }

func scanUser(r rowScanner) (domain.User, error) {
	var (
		u       domain.User
		created sql.NullInt64
	)
<<<<<<< Updated upstream
	err := r.Scan(&u.ID, &u.TelegramID, &u.Username, &u.Name, &u.PhotoURL, &created)
=======
	err := r.Scan(&u.ID, &u.TelegramChatID, &u.Username, &u.Name, &created)
>>>>>>> Stashed changes
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	u.CreatedAt = fromMS(created)
	return u, err
}

func (r *Users) UpsertByTelegram(ctx context.Context, p domain.TelegramProfile, now time.Time) (domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `
<<<<<<< Updated upstream
		INSERT INTO users (telegram_id, username, name, photo_url, created_at, last_login_at)
		VALUES (?1, ?2, ?3, ?4, ?5, ?5)
		ON CONFLICT (telegram_id) DO UPDATE
		   SET username = excluded.username, name = excluded.name,
		       photo_url = excluded.photo_url, last_login_at = excluded.last_login_at
		RETURNING `+userCols,
		id.TelegramID, id.Username, id.FullName(), id.PhotoURL, now.UnixMilli()))
=======
		INSERT INTO users (telegram_id, username, name, created_at, last_login_at)
		VALUES (?1, ?2, ?3, ?4, ?4)
		ON CONFLICT (telegram_id) DO UPDATE
		   SET username = excluded.username, name = excluded.name, last_login_at = excluded.last_login_at
		RETURNING `+userCols,
		p.ID, p.Username, p.Name, now.UnixMilli()))
>>>>>>> Stashed changes
}

func (r *Users) Get(ctx context.Context, id int64) (domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?1`, id))
}
