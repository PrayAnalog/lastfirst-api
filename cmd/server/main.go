package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"

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

	srv := httpapi.New(yt, cfg.StaticDir)
	httpServer := &http.Server{Addr: cfg.Addr, Handler: srv.Handler()}

	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM)
	defer stop()

	shutdownDone := make(chan error, 1)
	go func() {
		<-sigCtx.Done()
		shutdownDone <- httpServer.Shutdown(context.Background())
	}()

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	if err := <-shutdownDone; err != nil {
		log.Fatal(err)
	}
}
