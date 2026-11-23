package postgres

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
	_, err := r.db.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		tokenHash, userID, expiresAt)
	return err
}

func (r *Sessions) UserByToken(ctx context.Context, tokenHash []byte, now time.Time) (domain.User, error) {
	u, err := scanUser(r.db.QueryRowContext(ctx, `
		SELECT u.id, u.google_sub, u.email, u.name, u.avatar_url, COALESCE(u.telegram_chat_id, ''), u.created_at
		  FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1 AND s.expires_at > $2`, tokenHash, now))
	if errors.Is(err, domain.ErrNotFound) {
		return domain.User{}, domain.ErrUnauthorized
	}
	return u, err
}

func (r *Sessions) Delete(ctx context.Context, tokenHash []byte) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (r *Sessions) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= $1`, now)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if _, err := r.db.ExecContext(ctx, `DELETE FROM telegram_link_codes WHERE expires_at <= $1`, now); err != nil {
		return n, err
	}
	return n, nil
}

// LinkCodes — Telegram ulash uchun bir martalik kodlar.
type LinkCodes struct{ db *sql.DB }

var _ usecase.LinkCodeRepository = (*LinkCodes)(nil)

func NewLinkCodes(db *sql.DB) *LinkCodes { return &LinkCodes{db: db} }

func (r *LinkCodes) Create(ctx context.Context, code string, userID int64, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO telegram_link_codes (code, user_id, expires_at) VALUES ($1, $2, $3)`,
		code, userID, expiresAt)
	return err
}

func (r *LinkCodes) Consume(ctx context.Context, code string, now time.Time) (int64, error) {
	var userID int64
	err := r.db.QueryRowContext(ctx, `
		DELETE FROM telegram_link_codes WHERE code = $1 AND expires_at > $2 RETURNING user_id`, code, now).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, domain.ErrNotFound
	}
	return userID, err
}
