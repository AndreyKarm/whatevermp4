package main

import (
	"context"
	"errors"
	"liotom/whateverserver/internal/config"
	"liotom/whateverserver/internal/db"
	"liotom/whateverserver/internal/events"
	"liotom/whateverserver/internal/httpapi"
	"liotom/whateverserver/internal/rooms"
	"liotom/whateverserver/internal/storage"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	if err := cfg.Check(); err != nil {
		log.Fatal(err)
	}
	if cfg.PlaybackSecret == config.DefaultPlaybackSecret {
		log.Print("warning: PLAYBACK_SECRET is not set; playback URLs can be forged")
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	st, err := storage.New(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	if err := st.SetCORS(ctx, cfg.AllowedOrigins); err != nil {
		log.Printf("set bucket cors (browser uploads may fail): %v", err)
	}

	bus := events.NewBus()
	go events.Listen(ctx, cfg.DatabaseURL, bus)

	hub := rooms.NewHub(pool, bus)
	go hub.Run(ctx)

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: httpapi.NewRouter(pool, st, hub, bus, cfg),
	}

	go func() {
		log.Printf("api listening on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(sctx)
}
