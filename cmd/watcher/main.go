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
	"github.com/khasansayliev/watcher/internal/adapter/railway"
	"github.com/khasansayliev/watcher/internal/adapter/sqlite"
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
	db, err := sqlite.Open(ctx, dbDriver, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := sqlite.Migrate(ctx, db); err != nil {
		return err
	}
	users := sqlite.NewUsers(db)
	sessions := sqlite.NewSessions(db)
	logins := sqlite.NewLoginRequests(db)
	loginCodes := sqlite.NewLoginCodes(db)
	watchRepo := sqlite.NewWatches(db)
	checkLogs := sqlite.NewCheckLogs(db)
	pollingRepo := sqlite.NewPollingRules(db)
	settingsRepo := sqlite.NewSettings(db)

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

	// --- usecase'lar ---
	admin := usecase.NewAdminService(users, watchRepo, settingsRepo, sessions, cfg.AdminTelegramIDs)
	monitor := usecase.NewMonitor(watchRepo, checkLogs, pollingRepo, users, admin, rail, notifier, clk, usecase.MonitorConfig{
		Tick:                10 * time.Second,
		MaxBackoff:          30 * time.Minute,
		GroupPause:          2 * time.Second,
		MaxSearchesPerCycle: cfg.MaxSearchesPerCycle,
	}, log)
	auth := usecase.NewAuthService(users, logins, sessions, loginCodes, notifier, clk, cfg.SessionTTL).WithSiteURL(cfg.BaseURL)
	watches := usecase.NewWatchService(watchRepo, checkLogs, users, clk, monitor, admin, cfg.MaxWatchesPerUser)
	search := usecase.NewSearchService(rail, monitor, clk, 10*time.Second, 2*time.Second)
	tgService := usecase.NewTelegramService(users, notifier)
	polling := usecase.NewPollingService(pollingRepo, watchRepo, clk, cfg.AdminTelegramIDs)

	// --- delivery ---
	ui, err := webui.New(webui.Deps{
		Auth:          auth,
		Watches:       watches,
		Search:        search,
		Telegram:      tgService,
		Polling:       polling,
		Admin:         admin,
		Status:        monitor,
		Raw:           rail,
		Stations:      dir,
		Clock:         clk,
		TelegramOK:    notifier.Enabled(),
		MaxWatches:    cfg.MaxWatchesPerUser,
		AdminIDs:      cfg.AdminTelegramIDs,
		SecureCookies: cfg.SecureCookies(),
	})
	if err != nil {
		return err
	}
	bot := telegrambot.New(tgClient, auth, log)

	go monitor.Run(ctx)
	go bot.Run(ctx)
	go cleanupLoop(ctx, auth, watches, cfg.LogRetention, log)

	srv := &http.Server{Addr: cfg.Listen, Handler: ui.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	}()

	log.Info("ishga tushdi", "listen", cfg.Listen, "base_url", cfg.BaseURL, "db", cfg.DatabasePath)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Info("to'xtatildi")
	return nil
}

func cleanupLoop(ctx context.Context, auth *usecase.AuthService, watches *usecase.WatchService, logRetention time.Duration, log *slog.Logger) {
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
			if logRetention > 0 {
				if n, err := watches.PruneLogs(ctx, logRetention); err != nil {
					log.Warn("eski loglarni tozalash", "err", err)
				} else if n > 0 {
					log.Info("eski tekshiruv loglari o'chirildi", "count", n)
				}
			}
		}
	}
}
