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

func main() {
	ctx := context.Background()
	cfg := config.Load()

	yt, err := youtube.New(ctx, cfg.YTAPIKey)
	if err != nil {
		log.Fatal(err)
	}

	const writeTimeout = 2 * time.Minute
	const shutdownTimeout = writeTimeout + 10*time.Second

	srv := httpapi.New(yt, cfg.StaticDir)
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       90 * time.Second,
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	case <-sigCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Fatal(err)
		}
	}
}
