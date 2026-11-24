package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

type Sessions struct{ db *sql.DB }

var _ usecase.SessionRepository = (*Sessions)(nil)

func NewSessions(db *sql.DB) *Sessions { return &Sessions{db: db} }

func (r *Sessions) Create(ctx context.Context, tokenHash []byte, userID int64, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?1, ?2, ?3, ?4)`,
		tokenHash, userID, time.Now().UnixMilli(), expiresAt.UnixMilli())
	return err
}

func (r *Sessions) UserByToken(ctx context.Context, tokenHash []byte, now time.Time) (domain.User, error) {
	u, err := scanUser(r.db.QueryRowContext(ctx, `
		SELECT u.id, u.telegram_id, u.username, u.name, u.photo_url, u.created_at
		  FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = ?1 AND s.expires_at > ?2`, tokenHash, now.UnixMilli()))
	if errors.Is(err, domain.ErrNotFound) {
		return domain.User{}, domain.ErrUnauthorized
	}
	return u, err
}

func (r *Sessions) Delete(ctx context.Context, tokenHash []byte) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?1`, tokenHash)
	return err
}

func (r *Sessions) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?1`, now.UnixMilli())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
