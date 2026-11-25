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

func (r *LoginCodes) Put(ctx context.Context, hash []byte, id domain.Identity, expiresAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM login_codes WHERE telegram_id = ?1`, id.TelegramID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO login_codes (code_hash, telegram_id, first_name, last_name, username, created_at, expires_at)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)`,
		hash, id.TelegramID, id.FirstName, id.LastName, id.Username, time.Now().UnixMilli(), expiresAt.UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

// Consume bitta SQL buyrug'ida o'chiradi va qaytaradi — ikki so'rov bir kodni ikki marta ishlata olmaydi.
func (r *LoginCodes) Consume(ctx context.Context, hash []byte, now time.Time) (domain.Identity, error) {
	var id domain.Identity
	err := r.db.QueryRowContext(ctx, `
		DELETE FROM login_codes WHERE code_hash = ?1 AND expires_at > ?2
		RETURNING telegram_id, first_name, last_name, username`, hash, now.UnixMilli()).
		Scan(&id.TelegramID, &id.FirstName, &id.LastName, &id.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Identity{}, domain.ErrNotFound
	}
	return id, err
}

func (r *LoginCodes) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM login_codes WHERE expires_at <= ?1`, now.UnixMilli())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
