// Composition root: barcha qatlamlar shu yerda bir-biriga ulanadi.
// Bog'liqliklar yo'nalishi: delivery → usecase → domain; adapterlar usecase portlarini amalga oshiradi.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/khasansayliev/watcher/internal/adapter/clock"
	"github.com/khasansayliev/watcher/internal/adapter/google"
	"github.com/khasansayliev/watcher/internal/adapter/postgres"
	"github.com/khasansayliev/watcher/internal/adapter/railway"
	"github.com/khasansayliev/watcher/internal/adapter/stations"
	"github.com/khasansayliev/watcher/internal/adapter/telegram"
	"github.com/khasansayliev/watcher/internal/config"
	"github.com/khasansayliev/watcher/internal/delivery/telegrambot"
	"github.com/khasansayliev/watcher/internal/delivery/webui"
	"github.com/khasansayliev/watcher/internal/usecase"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(log)
	if err := run(log); err != nil {
		log.Error("ishga tushmadi", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	for _, w := range cfg.Warnings {
		log.Warn(w)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- adapterlar ---
	db, err := postgres.Open(ctx, dbDriver, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := postgres.Migrate(ctx, db); err != nil {
		return err
	}
	users := postgres.NewUsers(db)
	sessions := postgres.NewSessions(db)
	codes := postgres.NewLinkCodes(db)
	watchRepo := postgres.NewWatches(db)
	pollingRepo := postgres.NewPollingRules(db)

	clk := clock.New(cfg.TimeZone)
	dir, err := stations.Load(cfg.StationsFile)
	if err != nil {
		return err
	}
	_ = os.MkdirAll(cfg.DataDir, 0o755)
	rail := railway.New(railway.Config{
		BaseURL:      cfg.RailwayBaseURL,
		Lang:         cfg.RailwayLang,
		ManualCookie: cfg.RailwayCookie,
		RawDumpPath:  filepath.Join(cfg.DataDir, "last_response.json"),
	})
	tgClient := telegram.NewClient(cfg.TelegramToken)
	notifier := telegram.NewNotifier(tgClient, rail.BuyURL(), dir)
	idp := google.New(google.Config{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		RedirectURL:  cfg.GoogleRedirectURL(),
	})

	// --- usecase'lar ---
	monitor := usecase.NewMonitor(watchRepo, pollingRepo, users, rail, notifier, clk, usecase.MonitorConfig{
		Tick:                10 * time.Second,
		MaxBackoff:          30 * time.Minute,
		GroupPause:          2 * time.Second,
		MaxSearchesPerCycle: cfg.MaxSearchesPerCycle,
	}, log)
	auth := usecase.NewAuthService(idp, users, sessions, clk, cfg.SessionTTL)
	watches := usecase.NewWatchService(watchRepo, users, clk, monitor, cfg.MaxWatchesPerUser)
	search := usecase.NewSearchService(rail, monitor, clk, 10*time.Second, 2*time.Second)
	tgService := usecase.NewTelegramService(users, codes, notifier, clk)
	polling := usecase.NewPollingService(pollingRepo, watchRepo, clk, cfg.AdminEmails)

	// --- delivery ---
	ui, err := webui.New(webui.Deps{
		Auth:          auth,
		Watches:       watches,
		Search:        search,
		Telegram:      tgService,
		Polling:       polling,
		Status:        monitor,
		Raw:           rail,
		Stations:      dir,
		Clock:         clk,
		TelegramOK:    notifier.Enabled(),
		MaxWatches:    cfg.MaxWatchesPerUser,
		AdminEmails:   cfg.AdminEmails,
		SecureCookies: cfg.SecureCookies(),
	})
	if err != nil {
		return err
	}
	bot := telegrambot.New(tgClient, tgService, log)

	go monitor.Run(ctx)
	go bot.Run(ctx)
	go cleanupLoop(ctx, auth, log)

	srv := &http.Server{Addr: cfg.Listen, Handler: ui.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	}()

	log.Info("ishga tushdi", "listen", cfg.Listen, "base_url", cfg.BaseURL)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Info("to'xtatildi")
	return nil
}

func cleanupLoop(ctx context.Context, auth *usecase.AuthService, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := auth.CleanupExpired(ctx); err != nil {
				log.Warn("sessiyalarni tozalash", "err", err)
			} else if n > 0 {
				log.Info("eskirgan sessiyalar o'chirildi", "count", n)
			}
		}
	}
}
