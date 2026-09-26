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

const shutdownTimeout = 5 * time.Second

func main() {
	ctx := context.Background()
	cfg := config.Load()

	yt, err := youtube.New(ctx, cfg.YTAPIKey)
	if err != nil {
		log.Fatal(err)
	}

	srv := httpapi.New(yt, cfg.StaticDir)
	server := &http.Server{Addr: cfg.Addr, Handler: srv.Handler()}

	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM)
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()

	select {
	case err := <-serveErr:
		log.Fatal(err)
	case <-sigCtx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	shutdownErr := server.Shutdown(shutdownCtx)
	cancel()
	stop()

	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	if shutdownErr != nil {
		log.Fatal(shutdownErr)
	}
}
