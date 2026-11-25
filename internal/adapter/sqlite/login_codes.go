package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

type LoginCodes struct{ db *sql.DB }

var _ usecase.LoginCodeRepository = (*LoginCodes)(nil)

func NewLoginCodes(db *sql.DB) *LoginCodes { return &LoginCodes{db: db} }

func (r *LoginCodes) Put(ctx context.Context, hash []byte, p domain.TelegramProfile, expiresAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM login_codes WHERE telegram_id = ?1`, p.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO login_codes (code_hash, telegram_id, username, name, created_at, expires_at)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6)`,
		hash, p.ID, p.Username, p.Name, time.Now().UnixMilli(), expiresAt.UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

// Consume bitta SQL buyrug'ida o'chiradi va qaytaradi — ikki so'rov bir kodni ikki marta ishlata olmaydi.
func (r *LoginCodes) Consume(ctx context.Context, hash []byte, now time.Time) (domain.TelegramProfile, error) {
	var p domain.TelegramProfile
	err := r.db.QueryRowContext(ctx, `
		DELETE FROM login_codes WHERE code_hash = ?1 AND expires_at > ?2
		RETURNING telegram_id, username, name`, hash, now.UnixMilli()).
		Scan(&p.ID, &p.Username, &p.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.TelegramProfile{}, domain.ErrNotFound
	}
	return p, err
}

func (r *LoginCodes) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM login_codes WHERE expires_at <= ?1`, now.UnixMilli())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
