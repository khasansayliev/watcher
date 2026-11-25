package telegrambot

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/khasansayliev/watcher/internal/adapter/telegram"
	"github.com/khasansayliev/watcher/internal/domain"
)

type fakeSvc struct {
	greeted []string
	idents  []domain.Identity
}

func (f *fakeSvc) Greet(_ context.Context, chat string) error {
	f.greeted = append(f.greeted, chat)
	return nil
}
func (f *fakeSvc) SendLoginCode(_ context.Context, id domain.Identity) error {
	f.idents = append(f.idents, id)
	return nil
}

func TestParseCommand(t *testing.T) {
	cases := map[string][2]string{
		"/login":             {"/login", ""},
		"/LOGIN@WatcherBot":  {"/login", ""},
		"  /start login ":    {"/start", "login"},
		"/start":             {"/start", ""},
		"salom":              {"salom", ""},
		"/login@bot extra x": {"/login", "extra x"},
	}
	for in, want := range cases {
		if c, a := parseCommand(in); c != want[0] || a != want[1] {
			t.Errorf("%q -> %q %q, kutilgan %v", in, c, a, want)
		}
	}
}

func TestHandleRouting(t *testing.T) {
	svc := &fakeSvc{}
	b := New(nil, svc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	up := func(text string) telegram.Update {
		return telegram.Update{ChatID: "42", FromID: 42, FirstName: "Ali", Username: "ali", Text: text}
	}

	b.handle(context.Background(), up("/start"))
	b.handle(context.Background(), up("/start login"))
	b.handle(context.Background(), up("/login"))
	b.handle(context.Background(), up("/login@watcher_bot"))
	b.handle(context.Background(), up("boshqa matn"))
	b.handle(context.Background(), telegram.Update{Text: "/login"})               // shaxsiy chat emas — e'tiborsiz
	b.handle(context.Background(), telegram.Update{ChatID: "42", Text: "/login"}) // yuboruvchi noma'lum — e'tiborsiz

	if len(svc.greeted) != 1 {
		t.Fatalf("faqat oddiy /start salomlashishi kerak: %v", svc.greeted)
	}
	if len(svc.idents) != 3 {
		t.Fatalf("3 ta kod so'rovi kutilgan: %d", len(svc.idents))
	}
	if id := svc.idents[0]; id.TelegramID != 42 || id.Username != "ali" || id.FirstName != "Ali" {
		t.Fatalf("identity: %+v", id)
	}
}
