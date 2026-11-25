package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/khasansayliev/watcher/internal/domain"
)

func TestLoginCodes(t *testing.T) {
	ctx := context.Background()
	db := open(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	codes := NewLoginCodes(db)
	now := time.Now()
	p := domain.TelegramProfile{ID: "555", Username: "ali", Name: "Ali V"}

	if err := codes.Put(ctx, []byte("h1"), p, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := codes.Put(ctx, []byte("h2"), p, now.Add(time.Minute)); err != nil { // yangi kod eskisini bekor qiladi
		t.Fatal(err)
	}
	if _, err := codes.Consume(ctx, []byte("h1"), now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("eski kod bekor bo'lishi kerak: %v", err)
	}

	// Bir vaqtda 10 ta so'rov: kod faqat bir marta ishlashi kerak.
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := codes.Consume(ctx, []byte("h2"), now)
			if err != nil {
				return
			}
			if got != p {
				t.Errorf("profil: %+v", got)
			}
			mu.Lock()
			ok++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("kod %d marta ishladi, 1 marta bo'lishi kerak", ok)
	}

	if err := codes.Put(ctx, []byte("h3"), p, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := codes.Consume(ctx, []byte("h3"), now.Add(2*time.Minute)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("muddati o'tgan kod rad etilishi kerak: %v", err)
	}
	if n, err := codes.DeleteExpired(ctx, now.Add(2*time.Minute)); err != nil || n != 1 {
		t.Fatalf("tozalash: %d %v", n, err)
	}
}
