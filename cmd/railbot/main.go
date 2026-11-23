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

	"github.com/khasansayliev/railbot/internal/adapter/clock"
	"github.com/khasansayliev/railbot/internal/adapter/railway"
	"github.com/khasansayliev/railbot/internal/adapter/stations"
	"github.com/khasansayliev/railbot/internal/adapter/storage/jsonfile"
	"github.com/khasansayliev/railbot/internal/adapter/telegram"
	"github.com/khasansayliev/railbot/internal/config"
	"github.com/khasansayliev/railbot/internal/delivery/webui"
	"github.com/khasansayliev/railbot/internal/usecase"
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

	// --- adapterlar ---
	clk := clock.New(cfg.TimeZone)
	repo, err := jsonfile.Open(filepath.Join(cfg.DataDir, "watches.json"))
	if err != nil {
		return err
	}
	dir, err := stations.Load(cfg.StationsFile)
	if err != nil {
		return err
	}
	rail := railway.New(railway.Config{
		BaseURL:      cfg.RailwayBaseURL,
		Lang:         cfg.RailwayLang,
		ManualCookie: cfg.RailwayCookie,
		RawDumpPath:  filepath.Join(cfg.DataDir, "last_response.json"),
	})
	tg := telegram.New(cfg.TelegramToken, rail.BuyURL(), dir)

	// --- usecase'lar ---
	monitor := usecase.NewMonitor(repo, rail, tg, clk, usecase.MonitorConfig{
		Interval:   cfg.PollInterval,
		MaxBackoff: 30 * time.Minute,
		GroupPause: 2 * time.Second,
	}, log)
	watches := usecase.NewWatchService(repo, tg, clk, monitor)
	search := usecase.NewSearchService(rail, monitor, clk, cfg.ManualSearchGap)

	// --- delivery ---
	ui, err := webui.New(webui.Deps{
		Watches:       watches,
		Search:        search,
		Status:        monitor,
		Raw:           rail,
		Stations:      dir,
		Clock:         clk,
		TelegramOK:    tg.Enabled(),
		DefaultChatID: cfg.DefaultChatID,
		AuthUser:      cfg.UIUser,
		AuthPassword:  cfg.UIPassword,
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go monitor.Run(ctx)

	srv := &http.Server{Addr: cfg.Listen, Handler: ui.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	}()

	log.Info("UI ishga tushdi", "addr", "http://localhost"+cfg.Listen, "interval", cfg.PollInterval)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Info("to'xtatildi")
	return nil
}
