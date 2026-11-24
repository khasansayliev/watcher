package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/khasansayliev/watcher/internal/domain"
	"github.com/khasansayliev/watcher/internal/usecase"
)

func open(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(context.Background(), "sqlite", filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// Eski (Google) sxemadagi bazadan yangilash: Telegram ulangan foydalanuvchi va uning kuzatuvi
// saqlanishi, ulanmagan foydalanuvchi va sessiyalar o'chishi kerak.
func TestMigrateFromGoogleSchema(t *testing.T) {
	ctx := context.Background()
	db := open(t)
	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations (version TEXT PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"0001_init", "0002_polling_rules"} {
		body, _ := migrationsFS.ReadFile("migrations/" + v + ".sql")
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			t.Fatal(err)
		}
		db.ExecContext(ctx, `INSERT INTO schema_migrations VALUES (?1, 0)`, v)
	}
	mustExec(t, db, `INSERT INTO users (id, google_sub, email, name, telegram_chat_id, created_at, last_login_at) VALUES (1,'g1','a@b.c','Ali','555',1,1), (2,'g2','x@y.z','Vali',NULL,1,1)`)
	mustExec(t, db, `INSERT INTO sessions VALUES (x'01', 1, 1, 9999999999999)`)
	for _, uid := range []int{1, 2} {
		mustExec(t, db, `INSERT INTO watches (user_id, from_code, to_code, travel_date, created_at) VALUES (?1,'1','2','2026-12-05',1)`, uid)
	}

	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}

	var users, watches, sessions int
	db.QueryRow(`SELECT count(*) FROM users`).Scan(&users)
	db.QueryRow(`SELECT count(*) FROM watches`).Scan(&watches)
	db.QueryRow(`SELECT count(*) FROM sessions`).Scan(&sessions)
	if users != 1 || watches != 1 || sessions != 0 {
		t.Fatalf("users=%d watches=%d sessions=%d (kutilgan 1, 1, 0)", users, watches, sessions)
	}
	u, err := NewUsers(db).Get(ctx, 1)
	if err != nil || u.TelegramChatID != "555" || u.Name != "Ali" {
		t.Fatalf("eski foydalanuvchi saqlanmadi: %+v %v", u, err)
	}
	var fk int
	db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk)
	if fk != 1 {
		t.Fatal("migratsiyadan keyin foreign_keys yoqilgan bo'lishi kerak")
	}
}

func TestTelegramUsersAndLoginRequests(t *testing.T) {
	ctx := context.Background()
	db := open(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	users, logins, sessions := NewUsers(db), NewLoginRequests(db), NewSessions(db)
	now := time.Now()

	a, err := users.UpsertByTelegram(ctx, domain.TelegramProfile{ID: "555", Username: "ali", Name: "Ali"}, now)
	if err != nil {
		t.Fatal(err)
	}
	a2, _ := users.UpsertByTelegram(ctx, domain.TelegramProfile{ID: "555", Username: "ali2", Name: "Ali"}, now)
	if a2.ID != a.ID || a2.Username != "ali2" {
		t.Fatalf("upsert: %+v", a2)
	}

	if err := logins.Begin(ctx, "c1", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := logins.Claim(ctx, "c1", now); !errors.Is(err, usecase.ErrLoginPending) {
		t.Fatalf("pending kutilgan: %v", err)
	}
	if err := logins.Approve(ctx, "c1", a.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := logins.Approve(ctx, "c1", 999, now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ikkinchi tasdiqlash rad etilishi kerak: %v", err)
	}
	if id, err := logins.Claim(ctx, "c1", now); err != nil || id != a.ID {
		t.Fatalf("claim: %d %v", id, err)
	}
	if _, err := logins.Claim(ctx, "c1", now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("claim bir martalik")
	}

	logins.Begin(ctx, "c2", now.Add(time.Minute))
	if err := logins.Approve(ctx, "c2", a.ID, now.Add(2*time.Minute)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("eskirgan so'rov tasdiqlanmasligi kerak")
	}

	if err := sessions.Create(ctx, []byte("h"), a.ID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if u, err := sessions.UserByToken(ctx, []byte("h"), now); err != nil || u.TelegramChatID != "555" {
		t.Fatalf("sessiya: %+v %v", u, err)
	}
	if err := sessions.DeleteByTelegram(ctx, "555"); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.UserByToken(ctx, []byte("h"), now); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("sessiya o'chishi kerak")
	}
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}
