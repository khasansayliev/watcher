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
		SELECT u.id, u.telegram_id, u.username, u.name, u.created_at
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
	if _, err := r.db.ExecContext(ctx, `DELETE FROM login_requests WHERE expires_at <= ?1`, now.UnixMilli()); err != nil {
		return n, err
	}
	return n, nil
}

func (r *Sessions) DeleteByTelegram(ctx context.Context, telegramID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE telegram_id = ?1)`, telegramID)
	return err
}

// LoginRequests — Telegram orqali kirish uchun bir martalik so'rovlar.
type LoginRequests struct{ db *sql.DB }

var _ usecase.LoginRequestRepository = (*LoginRequests)(nil)

func NewLoginRequests(db *sql.DB) *LoginRequests { return &LoginRequests{db: db} }

func (r *LoginRequests) Begin(ctx context.Context, code string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO login_requests (code, expires_at) VALUES (?1, ?2)`, code, expiresAt.UnixMilli())
	return err
}

func (r *LoginRequests) Approve(ctx context.Context, code string, userID int64, now time.Time) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE login_requests SET user_id = ?2 WHERE code = ?1 AND user_id IS NULL AND expires_at > ?3`,
		code, userID, now.UnixMilli())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *LoginRequests) Claim(ctx context.Context, code string, now time.Time) (int64, error) {
	var userID int64
	err := r.db.QueryRowContext(ctx, `
		DELETE FROM login_requests WHERE code = ?1 AND user_id IS NOT NULL AND expires_at > ?2 RETURNING user_id`,
		code, now.UnixMilli()).Scan(&userID)
	if err == nil {
		return userID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var pending int
	err = r.db.QueryRowContext(ctx, `SELECT 1 FROM login_requests WHERE code = ?1 AND expires_at > ?2`, code, now.UnixMilli()).Scan(&pending)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, domain.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return 0, usecase.ErrLoginPending
}
