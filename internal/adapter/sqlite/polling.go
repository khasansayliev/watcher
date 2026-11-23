package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

type PollingRules struct{ db *sql.DB }

var _ usecase.PollingRuleRepository = (*PollingRules)(nil)

func NewPollingRules(db *sql.DB) *PollingRules { return &PollingRules{db: db} }

func (r *PollingRules) Get(ctx context.Context) (domain.PollingPolicy, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT max_days_left, interval_seconds, updated_at, updated_by
		  FROM polling_rules ORDER BY max_days_left = 0, max_days_left`)
	if err != nil {
		return domain.PollingPolicy{}, err
	}
	defer rows.Close()

	var (
		rules     []domain.PollingRule
		updatedAt time.Time
		updatedBy string
	)
	for rows.Next() {
		var (
			days, secs int
			at         sql.NullInt64
			by         string
		)
		if err := rows.Scan(&days, &secs, &at, &by); err != nil {
			return domain.PollingPolicy{}, err
		}
		rules = append(rules, domain.PollingRule{MaxDaysLeft: days, Interval: time.Duration(secs) * time.Second})
		if t := fromMS(at); t.After(updatedAt) {
			updatedAt, updatedBy = t, by
		}
	}
	if err := rows.Err(); err != nil {
		return domain.PollingPolicy{}, err
	}
	if len(rules) == 0 {
		return domain.DefaultPollingPolicy(), nil
	}
	p, err := domain.NewPollingPolicy(rules)
	if err != nil {
		return domain.DefaultPollingPolicy(), nil // bazada buzuq qiymat — xavfsiz standart
	}
	p.UpdatedAt, p.UpdatedBy = updatedAt, updatedBy
	return p, nil
}

// Save jadvalni bitta tranzaksiyada to'liq almashtiradi.
func (r *PollingRules) Save(ctx context.Context, p domain.PollingPolicy) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM polling_rules`); err != nil {
		return err
	}
	for _, rule := range p.Rules {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO polling_rules (max_days_left, interval_seconds, updated_at, updated_by)
			VALUES (?1, ?2, ?3, ?4)`,
			rule.MaxDaysLeft, int(rule.Interval/time.Second), p.UpdatedAt.UnixMilli(), p.UpdatedBy); err != nil {
			return err
		}
	}
	return tx.Commit()
}
