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

	var listenErr error
	select {
	case listenErr = <-serveErr:
	case <-sigCtx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	shutdownErr := server.Shutdown(shutdownCtx)
	cancel()
	stop()

	if listenErr == nil {
		listenErr = <-serveErr
	}
	if !errors.Is(listenErr, http.ErrServerClosed) {
		log.Fatal(listenErr)
	}
	if shutdownErr != nil {
		log.Fatal(shutdownErr)
	}
}
