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

const userCols = `id, telegram_id, username, name, photo_url, created_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanUser(r rowScanner) (domain.User, error) {
	var (
		u       domain.User
		created sql.NullInt64
	)
	err := r.Scan(&u.ID, &u.TelegramID, &u.Username, &u.Name, &u.PhotoURL, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	u.CreatedAt = fromMS(created)
	return u, err
}

func (r *Users) UpsertByIdentity(ctx context.Context, id domain.Identity, now time.Time) (domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `
		INSERT INTO users (telegram_id, username, name, photo_url, created_at, last_login_at)
		VALUES (?1, ?2, ?3, ?4, ?5, ?5)
		ON CONFLICT (telegram_id) DO UPDATE
		   SET username = excluded.username, name = excluded.name,
		       photo_url = CASE WHEN excluded.photo_url <> '' THEN excluded.photo_url ELSE users.photo_url END,
		       last_login_at = excluded.last_login_at
		RETURNING `+userCols,
		id.TelegramID, id.Username, id.FullName(), id.PhotoURL, now.UnixMilli()))
}

func (r *Users) Get(ctx context.Context, id int64) (domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?1`, id))
}
