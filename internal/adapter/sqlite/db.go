// Package sqlite — usecase saqlash portlarining SQLite amalga oshirilishi.
// Faqat database/sql ishlatiladi; drayver (modernc.org/sqlite, cgo'siz) composition root'da ulanadi.
//
// Konventsiyalar: parametrlar ?1, ?2 ... ko'rinishida; vaqtlar INTEGER (Unix millisekund, UTC),
// NULL — "hali bo'lmagan"; sanalar 'YYYY-MM-DD' matn.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DSN fayl yo'lidan drayver uchun ulanish satrini yasaydi:
// WAL (o'qish va yozish bir-biriga xalaqit bermaydi), foreign keys (ON DELETE CASCADE uchun), busy_timeout.
func DSN(path string) string {
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(NORMAL)")
	return "file:" + path + "?" + q.Encode()
}

// Open faylni ochadi (papkasini yaratadi) va ulanishni tekshiradi. driver: "sqlite".
func Open(ctx context.Context, driver, path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open(driver, DSN(path))
	if err != nil {
		return nil, err
	}
	// SQLite bir vaqtda bitta yozuvchiga ruxsat beradi. Bitta ulanish — "database is locked"
	// xatolarisiz eng sodda va ishonchli yo'l; bu platforma hajmi uchun yetarli tez.
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)

	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite ochilmadi (%s): %w", path, err)
	}
	return db, nil
}

// Migrate ichki SQL migratsiyalarni tartib bilan, har birini bir marta, tranzaksiyada qo'llaydi.
func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		return err
	}
	names, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		version := strings.TrimSuffix(strings.TrimPrefix(name, "migrations/"), ".sql")
		if err := applyOne(ctx, db, version, name); err != nil {
			return fmt.Errorf("migratsiya %s: %w", version, err)
		}
	}
	return nil
}

func applyOne(ctx context.Context, db *sql.DB, version, file string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE version = ?1`, version).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	body, err := migrationsFS.ReadFile(file)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?1, ?2)`,
		version, time.Now().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- vaqt va NULL yordamchilari ----

// ms vaqtni Unix millisekundga o'giradi; nol vaqt — NULL.
func ms(t time.Time) sql.NullInt64 {
	if t.IsZero() {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: t.UnixMilli(), Valid: true}
}

// fromMS Unix millisekundni vaqtga o'giradi; NULL yoki 0 — nol vaqt.
func fromMS(v sql.NullInt64) time.Time {
	if !v.Valid || v.Int64 == 0 {
		return time.Time{}
	}
	return time.UnixMilli(v.Int64)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
