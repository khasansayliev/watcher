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

const userCols = `id, telegram_id, username, name, created_at, can_create, disabled`

type rowScanner interface{ Scan(dest ...any) error }

func scanUser(r rowScanner) (domain.User, error) {
	var (
		u                 domain.User
		created           sql.NullInt64
		canCreate, disabl int
	)
	err := r.Scan(&u.ID, &u.TelegramChatID, &u.Username, &u.Name, &created, &canCreate, &disabl)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	u.CreatedAt = fromMS(created)
	u.CanCreate, u.Disabled = canCreate == 1, disabl == 1
	return u, err
}

func (r *Users) UpsertByTelegram(ctx context.Context, p domain.TelegramProfile, now time.Time) (domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `
		INSERT INTO users (telegram_id, username, name, created_at, last_login_at)
		VALUES (?1, ?2, ?3, ?4, ?4)
		ON CONFLICT (telegram_id) DO UPDATE
		   SET username = excluded.username, name = excluded.name, last_login_at = excluded.last_login_at
		RETURNING `+userCols,
		p.ID, p.Username, p.Name, now.UnixMilli()))
}

func (r *Users) Get(ctx context.Context, id int64) (domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?1`, id))
}

func (r *Users) List(ctx context.Context, limit, offset int) ([]domain.UserSummary, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT u.id, u.telegram_id, u.username, u.name, u.created_at, u.can_create, u.disabled, u.last_login_at,
		       (SELECT count(*) FROM watches w WHERE w.user_id = u.id AND w.active = 1 AND w.admin_disabled = 0),
		       (SELECT count(*) FROM watches w WHERE w.user_id = u.id)
		  FROM users u ORDER BY u.created_at DESC, u.id DESC LIMIT ?1 OFFSET ?2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.UserSummary
	for rows.Next() {
		var (
			s                   domain.UserSummary
			created, lastLogin  sql.NullInt64
			canCreate, disabled int
		)
		if err := rows.Scan(&s.ID, &s.TelegramChatID, &s.Username, &s.Name, &created, &canCreate, &disabled,
			&lastLogin, &s.ActiveWatches, &s.TotalWatches); err != nil {
			return nil, 0, err
		}
		s.CreatedAt, s.LastLoginAt = fromMS(created), fromMS(lastLogin)
		s.CanCreate, s.Disabled = canCreate == 1, disabled == 1
		out = append(out, s)
	}
	return out, total, rows.Err()
}

func (r *Users) SetCanCreate(ctx context.Context, id int64, allow bool) error {
	return r.setFlag(ctx, `UPDATE users SET can_create = ?2 WHERE id = ?1`, id, allow)
}

func (r *Users) SetDisabled(ctx context.Context, id int64, disabled bool) error {
	return r.setFlag(ctx, `UPDATE users SET disabled = ?2 WHERE id = ?1`, id, disabled)
}

func (r *Users) setFlag(ctx context.Context, q string, id int64, v bool) error {
	res, err := r.db.ExecContext(ctx, q, id, boolInt(v))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Settings — platforma sozlamalari (key/value).
type Settings struct{ db *sql.DB }

var _ usecase.SettingsRepository = (*Settings)(nil)

func NewSettings(db *sql.DB) *Settings { return &Settings{db: db} }

func (r *Settings) Get(ctx context.Context) (domain.Settings, error) {
	s := domain.DefaultSettings()
	var v string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'creation_mode'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if m := domain.CreationMode(v); m.Valid() {
		s.CreationMode = m
	}
	return s, nil
}

func (r *Settings) SetCreationMode(ctx context.Context, m domain.CreationMode) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO settings (key, value) VALUES ('creation_mode', ?1)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, string(m))
	return err
}
