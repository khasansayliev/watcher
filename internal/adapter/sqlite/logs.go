package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

type CheckLogs struct{ db *sql.DB }

var _ usecase.CheckLogRepository = (*CheckLogs)(nil)

func NewCheckLogs(db *sql.DB) *CheckLogs { return &CheckLogs{db: db} }

func (r *CheckLogs) Add(ctx context.Context, l domain.CheckLog) error {
	trains, err := json.Marshal(l.Trains)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO watch_logs (watch_id, req_time, duration_ms, ok, error, train_count, match_count,
		                        notified, notify_err, manual, trains)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11)`,
		l.WatchID, l.ReqTime.UnixMilli(), l.Duration.Milliseconds(), boolInt(l.OK), l.Error, l.TrainCount, l.MatchCount,
		boolInt(l.Notified), l.NotifyErr, boolInt(l.Manual), string(trains))
	return err
}

// ListByWatch req_time bo'yicha kamayish tartibida (bir xil vaqtda — id bo'yicha).
func (r *CheckLogs) ListByWatch(ctx context.Context, watchID int64, limit, offset int) ([]domain.CheckLog, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM watch_logs WHERE watch_id = ?1`, watchID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, watch_id, req_time, duration_ms, ok, error, train_count, match_count, notified, notify_err, manual, trains
		  FROM watch_logs WHERE watch_id = ?1
		 ORDER BY req_time DESC, id DESC LIMIT ?2 OFFSET ?3`, watchID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.CheckLog
	for rows.Next() {
		var (
			l                    domain.CheckLog
			req, dur             int64
			ok, notified, manual int
			trains               string
		)
		if err := rows.Scan(&l.ID, &l.WatchID, &req, &dur, &ok, &l.Error, &l.TrainCount, &l.MatchCount,
			&notified, &l.NotifyErr, &manual, &trains); err != nil {
			return nil, 0, err
		}
		l.ReqTime, l.Duration = time.UnixMilli(req), time.Duration(dur)*time.Millisecond
		l.OK, l.Notified, l.Manual = ok == 1, notified == 1, manual == 1
		_ = json.Unmarshal([]byte(trains), &l.Trains) // buzilgan yozuv ro'yxatni to'xtatmasin
		out = append(out, l)
	}
	return out, total, rows.Err()
}

func (r *CheckLogs) DeleteBefore(ctx context.Context, t time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM watch_logs WHERE req_time < ?1`, t.UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
