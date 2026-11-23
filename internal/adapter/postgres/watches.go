package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

type Watches struct{ db *sql.DB }

var _ usecase.WatchRepository = (*Watches)(nil)

func NewWatches(db *sql.DB) *Watches { return &Watches{db: db} }

const watchCols = `id, user_id, from_code, to_code, to_char(travel_date, 'YYYY-MM-DD'),
	max_price::float8, min_seats, car_type, active, created_at,
	last_check, last_status, last_matches, last_sig, last_notified`

func scanWatch(r rowScanner) (domain.Watch, error) {
	var (
		w                       domain.Watch
		lastCheck, lastNotified sql.NullTime
	)
	err := r.Scan(&w.ID, &w.UserID, &w.Route.From, &w.Route.To, &w.Route.Date,
		&w.Filter.MaxPrice, &w.Filter.MinSeats, &w.Filter.CarType, &w.Active, &w.CreatedAt,
		&lastCheck, &w.LastStatus, &w.LastMatches, &w.LastSig, &lastNotified)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Watch{}, domain.ErrNotFound
	}
	w.LastCheck, w.LastNotified = lastCheck.Time, lastNotified.Time
	return w, err
}

func (r *Watches) Create(ctx context.Context, w domain.Watch) (domain.Watch, error) {
	return scanWatch(r.db.QueryRowContext(ctx, `
		INSERT INTO watches (user_id, from_code, to_code, travel_date, max_price, min_seats, car_type, active, created_at)
		VALUES ($1, $2, $3, $4::date, $5, $6, $7, $8, $9)
		RETURNING `+watchCols,
		w.UserID, w.Route.From, w.Route.To, w.Route.Date, w.Filter.MaxPrice, w.Filter.MinSeats, w.Filter.CarType, w.Active, w.CreatedAt))
}

func (r *Watches) Get(ctx context.Context, id int64) (domain.Watch, error) {
	return scanWatch(r.db.QueryRowContext(ctx, `SELECT `+watchCols+` FROM watches WHERE id = $1`, id))
}

func (r *Watches) ListByUser(ctx context.Context, userID int64) ([]domain.Watch, error) {
	return r.list(ctx, `SELECT `+watchCols+` FROM watches WHERE user_id = $1 ORDER BY created_at DESC LIMIT 200`, userID)
}

func (r *Watches) ListActive(ctx context.Context) ([]domain.Watch, error) {
	return r.list(ctx, `SELECT `+watchCols+` FROM watches WHERE active ORDER BY last_check NULLS FIRST`)
}

func (r *Watches) CountActiveByUser(ctx context.Context, userID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM watches WHERE user_id = $1 AND active`, userID).Scan(&n)
	return n, err
}

func (r *Watches) Update(ctx context.Context, id int64, fn func(*domain.Watch)) (domain.Watch, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Watch{}, err
	}
	defer tx.Rollback()

	w, err := scanWatch(tx.QueryRowContext(ctx, `SELECT `+watchCols+` FROM watches WHERE id = $1 FOR UPDATE`, id))
	if err != nil {
		return domain.Watch{}, err
	}
	fn(&w)
	_, err = tx.ExecContext(ctx, `
		UPDATE watches
		   SET active = $2, last_check = $3, last_status = $4, last_matches = $5,
		       last_sig = $6, last_notified = $7
		 WHERE id = $1`,
		id, w.Active, nullTime(w.LastCheck), w.LastStatus, w.LastMatches, w.LastSig, nullTime(w.LastNotified))
	if err != nil {
		return domain.Watch{}, err
	}
	return w, tx.Commit()
}

func (r *Watches) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM watches WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *Watches) list(ctx context.Context, q string, args ...any) ([]domain.Watch, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Watch
	for rows.Next() {
		w, err := scanWatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
