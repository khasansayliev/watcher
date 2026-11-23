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

const userCols = `id, google_sub, email, name, avatar_url, COALESCE(telegram_chat_id, ''), created_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanUser(r rowScanner) (domain.User, error) {
	var (
		u       domain.User
		created sql.NullInt64
	)
	err := r.Scan(&u.ID, &u.GoogleSub, &u.Email, &u.Name, &u.AvatarURL, &u.TelegramChatID, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	u.CreatedAt = fromMS(created)
	return u, err
}

func (r *Users) UpsertByIdentity(ctx context.Context, id domain.Identity, now time.Time) (domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `
		INSERT INTO users (google_sub, email, name, avatar_url, created_at, last_login_at)
		VALUES (?1, ?2, ?3, ?4, ?5, ?5)
		ON CONFLICT (google_sub) DO UPDATE
		   SET email = excluded.email, name = excluded.name,
		       avatar_url = excluded.avatar_url, last_login_at = excluded.last_login_at
		RETURNING `+userCols,
		id.Subject, id.Email, id.Name, id.Picture, now.UnixMilli()))
}

func (r *Users) Get(ctx context.Context, id int64) (domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?1`, id))
}

func (r *Users) SetTelegramChat(ctx context.Context, userID int64, chatID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if chatID != "" {
		// Bitta chat faqat bitta akkauntga bog'lanadi.
		if _, err := tx.ExecContext(ctx, `UPDATE users SET telegram_chat_id = NULL WHERE telegram_chat_id = ?1 AND id <> ?2`, chatID, userID); err != nil {
			return err
		}
	}
	res, err := tx.ExecContext(ctx, `UPDATE users SET telegram_chat_id = ?2 WHERE id = ?1`, userID, nullString(chatID))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return tx.Commit()
}
