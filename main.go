package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	_ "time/tzdata" // Asia/Tashkent vaqt zonasi binar ichida bo'lishi uchun
)

func main() {
	cfg := loadConfig()

	store, err := NewStore(filepath.Join(cfg.DataDir, "watches.json"))
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	stations, err := loadStations(cfg.StationsFile)
	if err != nil {
		log.Fatalf("stations: %v", err)
	}

	rc := NewRailwayClient(cfg.RailwayBase, cfg.RailwayCookie, cfg.Lang, cfg.DataDir)
	tg := NewTelegram(cfg.BotToken)
	watcher := NewWatcher(store, rc, tg, stations, cfg.PollInterval, cfg.RailwayBase+"/"+cfg.Lang+"/")
	web := NewWeb(cfg, store, watcher, rc, tg, stations)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go watcher.Run(ctx)

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           web.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	}()

	log.Printf("UI: http://localhost%s  | tekshiruv oralig'i: %s", cfg.Listen, cfg.PollInterval)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	log.Println("to'xtatildi")
}
