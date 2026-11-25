package telegrambot

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/khasansayliev/watcher/internal/adapter/telegram"
	"github.com/khasansayliev/watcher/internal/domain"
)

type fakeLogin struct {
	greeted   []string
	codes     []domain.TelegramProfile
	confirmed []string // tugma oqimi: "/start <kod>"
}

func (f *fakeLogin) ConfirmLogin(_ context.Context, code string, _ domain.TelegramProfile) error {
	f.confirmed = append(f.confirmed, code)
	return nil
}
func (f *fakeLogin) Greet(_ context.Context, chat string) error {
	f.greeted = append(f.greeted, chat)
	return nil
}
func (f *fakeLogin) SendLoginCode(_ context.Context, p domain.TelegramProfile) error {
	f.codes = append(f.codes, p)
	return nil
}
func (f *fakeLogin) LogoutAll(context.Context, string) error { return nil }

func TestHandleRouting(t *testing.T) {
	f := &fakeLogin{}
	b := New(nil, f, slog.New(slog.NewTextHandler(io.Discard, nil)))
	up := func(text string) telegram.Update {
		return telegram.Update{ChatID: "42", Username: "ali", Name: "Ali", Text: text}
	}
	for _, text := range []string{
		"/start",             // salom
		"/start login",       // saytdagi "kod olish" havolasi → kod
		"/login",             // kod
		"/login@watcher_bot", // kod
		"/start abc123XYZ",   // tugma oqimi: kirish so'rovini tasdiqlash
		"boshqa matn",        // e'tiborsiz
	} {
		b.handle(context.Background(), up(text))
	}
	b.handle(context.Background(), telegram.Update{Text: "/login"}) // shaxsiy chat emas — e'tiborsiz

	if len(f.greeted) != 1 {
		t.Fatalf("faqat oddiy /start salomlashishi kerak: %v", f.greeted)
	}
	if len(f.confirmed) != 1 || f.confirmed[0] != "abc123XYZ" {
		t.Fatalf("tugma oqimi buzilmasligi kerak: %v", f.confirmed)
	}
	if len(f.codes) != 3 { // "/start login", "/login", "/login@watcher_bot"
		t.Fatalf("3 ta kod so'rovi kutilgan: %d", len(f.codes))
	}
	if p := f.codes[0]; p.ID != "42" || p.Username != "ali" || p.Name != "Ali" {
		t.Fatalf("profil: %+v", p)
	}
}
