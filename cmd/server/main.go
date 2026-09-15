package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"ytreverse/internal/config"
	"ytreverse/internal/httpapi"
	"ytreverse/internal/youtube"
)

// shutdownTimeout bounds draining so the process still exits inside
// Kubernetes' default 30s termination grace period.
const shutdownTimeout = 25 * time.Second

func main() {
	ctx := context.Background()
	cfg := config.Load()

	yt, err := youtube.New(ctx, cfg.YTAPIKey)
	if err != nil {
		log.Fatal(err)
	}

	api := httpapi.New(yt, cfg.StaticDir)
	srv := &http.Server{Addr: cfg.Addr, Handler: api.Handler()}

	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-sigCtx.Done()
	stop()

	shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Print(err)
	}
}
